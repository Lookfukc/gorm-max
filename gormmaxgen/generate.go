// Package gormmaxgen 为模型生成类型化字段集声明(M5b,反射模式)。
//
// 用法:在用户项目里写一个生成入口,纳入 go:generate:
//
//	//go:build ignore
//
//	package main
//
//	import (
//		"gorm-max/gormmaxgen"
//		"yourapp/model"
//	)
//
//	func main() {
//		if err := gormmaxgen.Generate(gormmaxgen.Config{
//			Out:    "query/fields_gen.go", // 相对运行时工作目录
//			Models: []any{&model.User{}, &model.Order{}},
//		}); err != nil {
//			panic(err)
//		}
//	}
//
// 列名来自 gorm schema.Parse,与运行时同一命名策略(传同一个 Namer 实例可保证不漂移)。
// 输出确定性:模型按类型名排序、字段按结构体声明顺序;输出经 gofmt 格式化。
package gormmaxgen

import (
	"errors"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"

	"gorm.io/gorm/schema"
)

// Config 生成配置。
type Config struct {
	// Out 输出文件路径(相对运行时工作目录),默认 "query/fields_gen.go"。
	Out string
	// Package 生成包名,默认 "query"。不要与模型包同名(自定义类型字段会导入模型包)。
	Package string
	// Models 模型实例或指针,必须是命名类型。
	Models []any
	// Namer 列名命名策略;nil 用默认 snake_case。
	Namer schema.Namer
	// Import gorm_max 包的导入路径,默认 "gorm-max"。
	Import string
}

type genField struct {
	name  string // Go 字段名
	col   string // DB 列名
	ftype string // 字段句柄类型表达式
	ctor  string // 构造表达式(含列名)
}

type genModel struct {
	name   string
	table  string
	typeT  reflect.Type
	fields []genField
}

// Generate 解析模型并写出字段集文件。同名不同类型的模型会报错;同一类型重复传入会被忽略。
func Generate(cfg Config) error {
	src, err := build(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Out), 0o755); err != nil {
		return fmt.Errorf("gormmaxgen: 创建输出目录: %w", err)
	}
	if err := os.WriteFile(cfg.Out, src, 0o644); err != nil {
		return fmt.Errorf("gormmaxgen: 写出文件: %w", err)
	}
	return nil
}

// build 解析模型并渲染源码(经 gofmt 格式化)。
func build(cfg Config) ([]byte, error) {
	if len(cfg.Models) == 0 {
		return nil, errors.New("gormmaxgen: Models 不能为空")
	}
	if cfg.Package == "" {
		cfg.Package = "query"
	}
	if cfg.Out == "" {
		cfg.Out = filepath.Join(cfg.Package, "fields_gen.go")
	}
	if cfg.Import == "" {
		cfg.Import = "gorm-max"
	}
	namer := cfg.Namer
	if namer == nil {
		namer = schema.NamingStrategy{}
	}

	byName := map[string]*genModel{}
	seenTypes := map[reflect.Type]bool{}
	imports := map[string]string{} // 模型包短名 -> 导入路径
	store := &sync.Map{}

	for _, m := range cfg.Models {
		if m == nil {
			return nil, errors.New("gormmaxgen: Models 含 nil")
		}
		t := reflect.TypeOf(m)
		for t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		if seenTypes[t] {
			continue // 同一模型重复传入,忽略
		}
		seenTypes[t] = true

		s, err := schema.Parse(reflect.New(t).Interface(), store, namer)
		if err != nil {
			return nil, fmt.Errorf("gormmaxgen: 解析模型 %v: %w", t, err)
		}
		if s.Name == "" {
			return nil, errors.New("gormmaxgen: 模型必须是命名类型,不能是匿名结构体")
		}
		if prev, ok := byName[s.Name]; ok {
			return nil, fmt.Errorf("gormmaxgen: 模型类型名重复: %s(%v 与 %v)", s.Name, prev.typeT, t)
		}

		gm := &genModel{name: s.Name, table: s.Table, typeT: t}
		for _, f := range s.Fields {
			if f.DBName == "" {
				continue // 关联等无列字段
			}
			ftype, ctor, fimports, err := mapField(f)
			if err != nil {
				return nil, fmt.Errorf("gormmaxgen: 模型 %s 字段 %s: %w", s.Name, f.Name, err)
			}
			gm.fields = append(gm.fields, genField{name: f.Name, col: f.DBName, ftype: ftype, ctor: ctor})
			for short, path := range fimports {
				if prev, ok := imports[short]; ok && prev != path {
					return nil, fmt.Errorf("gormmaxgen: 模型包短名冲突(%s): %s 与 %s", short, prev, path)
				}
				imports[short] = path
			}
		}
		byName[s.Name] = gm
	}

	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)

	src, err := format.Source([]byte(render(cfg, byName, names, imports)))
	if err != nil {
		return nil, fmt.Errorf("gormmaxgen: 渲染结果未通过 gofmt(生成器缺陷): %w", err)
	}
	return src, nil
}

var predeclaredKinds = map[reflect.Kind]string{
	reflect.String:  "StringField",
	reflect.Bool:    "BoolField",
	reflect.Int:     "IntField",
	reflect.Int8:    "Int8Field",
	reflect.Int16:   "Int16Field",
	reflect.Int32:   "Int32Field",
	reflect.Int64:   "Int64Field",
	reflect.Uint:    "UintField",
	reflect.Uint8:   "Uint8Field",
	reflect.Uint16:  "Uint16Field",
	reflect.Uint32:  "Uint32Field",
	reflect.Uint64:  "Uint64Field",
	reflect.Float32: "Float32Field",
	reflect.Float64: "Float64Field",
}

var predeclaredTypes = map[reflect.Kind]reflect.Type{
	reflect.String:  reflect.TypeOf(""),
	reflect.Bool:    reflect.TypeOf(false),
	reflect.Int:     reflect.TypeOf(int(0)),
	reflect.Int8:    reflect.TypeOf(int8(0)),
	reflect.Int16:   reflect.TypeOf(int16(0)),
	reflect.Int32:   reflect.TypeOf(int32(0)),
	reflect.Int64:   reflect.TypeOf(int64(0)),
	reflect.Uint:    reflect.TypeOf(uint(0)),
	reflect.Uint8:   reflect.TypeOf(uint8(0)),
	reflect.Uint16:  reflect.TypeOf(uint16(0)),
	reflect.Uint32:  reflect.TypeOf(uint32(0)),
	reflect.Uint64:  reflect.TypeOf(uint64(0)),
	reflect.Float32: reflect.TypeOf(float32(0)),
	reflect.Float64: reflect.TypeOf(float64(0)),
}

// mapField 把模型字段映射为字段句柄类型与构造表达式。
// 优先级:GORM 语义(time/bytes)→ 预声明 Go 类型 → 命名自定义类型(NewTyped,精确到原类型)
// → GenericField 兜底(unnamed 复合类型、Null 结构体、本地匿名类型等)。
func mapField(f *schema.Field) (ftype, ctor string, imports map[string]string, err error) {
	col := f.DBName
	switch f.GORMDataType {
	case schema.Time:
		return "gorm_max.TimeField", fmt.Sprintf("gorm_max.NewTime(%q)", col), nil, nil
	case schema.Bytes:
		return "gorm_max.BytesField", fmt.Sprintf("gorm_max.NewBytes(%q)", col), nil, nil
	}

	ft := f.IndirectFieldType
	if want, ok := predeclaredTypes[ft.Kind()]; ok && ft == want {
		name := predeclaredKinds[ft.Kind()]
		ctorName := strings.TrimSuffix(name, "Field") // StringField → NewString
		return "gorm_max." + name, fmt.Sprintf("gorm_max.New%s(%q)", ctorName, col), nil, nil
	}

	// 命名自定义类型:精确到原类型,值类型完全编译期校验
	if ft.Name() != "" && ft.PkgPath() != "" {
		typeExpr := ft.String() // 如 model.Status,与导入别名(包短名)一致
		if !strings.ContainsAny(typeExpr, "[]") {
			short := typeExpr[:strings.Index(typeExpr, ".")]
			return "gorm_max.TypedField[" + typeExpr + "]",
				fmt.Sprintf("gorm_max.NewTyped[%s](%q)", typeExpr, col),
				map[string]string{short: ft.PkgPath()}, nil
		}
	}

	return "gorm_max.GenericField", fmt.Sprintf("gorm_max.NewAny(%q)", col), nil, nil
}

func render(cfg Config, byName map[string]*genModel, names []string, imports map[string]string) string {
	var b strings.Builder
	b.WriteString("// Code generated by gormmaxgen. DO NOT EDIT.\n")
	b.WriteString("//\n")
	b.WriteString("// 列名由 gorm schema 解析生成,与运行时命名策略一致;模型变更后请重新生成。\n\n")
	b.WriteString("package " + cfg.Package + "\n\n")
	b.WriteString("import (\n")
	b.WriteString("\tgorm_max " + strconv.Quote(cfg.Import) + "\n")

	shorts := make([]string, 0, len(imports))
	for short := range imports {
		shorts = append(shorts, short)
	}
	sort.Strings(shorts)
	for _, short := range shorts {
		fmt.Fprintf(&b, "\t%s %s\n", short, strconv.Quote(imports[short]))
	}
	b.WriteString(")\n\n")

	for _, name := range names {
		gm := byName[name]
		fmt.Fprintf(&b, "// %sFields %s 的类型化字段集(表 %s)。\n", gm.name, gm.name, gm.table)
		fmt.Fprintf(&b, "type %sFields struct {\n", gm.name)
		for _, f := range gm.fields {
			fmt.Fprintf(&b, "\t%s %s\n", f.name, f.ftype)
		}
		fmt.Fprintf(&b, "}\n\n")
		fmt.Fprintf(&b, "// %s %s 的字段集实例。\n", gm.name, gm.name)
		fmt.Fprintf(&b, "var %s = %sFields{\n", gm.name, gm.name)
		for _, f := range gm.fields {
			fmt.Fprintf(&b, "\t%s: %s,\n", f.name, f.ctor)
		}
		fmt.Fprintf(&b, "}\n\n")
	}
	return b.String()
}
