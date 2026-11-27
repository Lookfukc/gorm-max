package gorm_max

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"gorm.io/gorm/schema"
)

// Column 字段选择器解析结果。
type Column struct {
	Name      string       // DB 列名,如 nick_name
	FieldName string       // Go 字段名,如 NickName
	FieldType reflect.Type // 字段 Go 类型
}

// resolver 每个模型类型构建一次,缓存复用。
type resolver struct {
	modelType reflect.Type
	table     string
	size      uintptr
	fields    []*schema.Field // 仅含列字段(DBName 非空)
	pk        *schema.Field
	offsets   map[uintptr]*schema.Field // 字段在结构体内的偏移 -> 字段(与分配无关,指针写法 O(1) 匹配)
}

type resolverKey struct {
	t     reflect.Type
	namer reflect.Type
}

var resolverCache sync.Map // resolverKey -> *resolver

func resolverOf[T any](namer schema.Namer) *resolver {
	var zero T
	t := reflect.TypeOf(zero)
	for t != nil && t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t == nil {
		panic("gorm_max: 无法确定模型类型,泛型参数不能是 nil 接口")
	}
	key := resolverKey{t: t}
	if namer != nil {
		key.namer = reflect.TypeOf(namer)
	}
	if v, ok := resolverCache.Load(key); ok {
		return v.(*resolver)
	}
	r := buildResolver(t, namer)
	resolverCache.Store(key, r)
	return r
}

func buildResolver(t reflect.Type, namer schema.Namer) *resolver {
	if namer == nil {
		namer = schema.NamingStrategy{}
	}
	s, err := schema.Parse(reflect.New(t).Interface(), &sync.Map{}, namer)
	if err != nil {
		panic(fmt.Sprintf("gorm_max: 解析模型 %v 失败: %v", t, err))
	}
	r := &resolver{modelType: t, table: s.Table, size: t.Size(), pk: s.PrioritizedPrimaryField, offsets: map[uintptr]*schema.Field{}}
	v := reflect.New(t).Elem()
	ctx := context.Background()
	base := v.Addr().Pointer()
	for _, f := range s.Fields {
		if f.DBName == "" { // 跳过关联等无列字段
			continue
		}
		r.fields = append(r.fields, f)
		// 偏移落在结构体内才登记;嵌入指针指向的外部分配由差分识别兜底
		if fv := f.ReflectValueOf(ctx, v); fv.IsValid() && fv.CanAddr() {
			if off := fv.Addr().Pointer(); off >= base && off-base < r.size {
				r.offsets[off-base] = f // 零大小字段偏移可能重合,后者覆盖,可忽略
			}
		}
	}
	return r
}

// resolve 把字段选择器闭包解析成列。
// 两种闭包写法:
//
//	func(u *User) any { return &u.Name }  指针写法,地址直接命中,O(1)
//	func(u *User) any { return u.Name }   值写法,逐字段差分识别
func (r *resolver) resolve(sel any) *Column {
	if sel == nil {
		panic("gorm_max: 字段选择器不能为空")
	}
	sv := reflect.ValueOf(sel)
	if sv.Kind() != reflect.Func || sv.Type().NumIn() != 1 || sv.Type().NumOut() != 1 {
		panic("gorm_max: 字段选择器必须是 func(*T) X 形式的闭包")
	}
	if in := sv.Type().In(0); in.Kind() != reflect.Ptr || in.Elem() != r.modelType {
		panic(fmt.Sprintf("gorm_max: 字段选择器参数 %v 与模型 %v 不匹配", in, r.modelType))
	}
	// 用自己的临时结构体让闭包"指认"字段
	temp := reflect.New(r.modelType)
	if out, ok := safeCall(sv, temp); ok {
		if out.Kind() == reflect.Interface && !out.IsNil() {
			out = out.Elem() // 闭包返回类型是 any,先取具体值
		}
		if out.Kind() == reflect.Ptr && !out.IsNil() && out.Elem().Kind() != reflect.Struct {
			// 指针写法:返回 &u.Name,按结构体内相对偏移匹配,与具体分配无关
			if off := out.Pointer() - temp.Pointer(); off < r.size {
				if f, hit := r.offsets[off]; hit {
					return columnOf(f)
				}
			}
		}
	}
	if f := r.diffMatch(sel); f != nil {
		return columnOf(f)
	}
	panic("gorm_max: 无法识别字段选择器,请确认闭包读写的是模型自身的列字段")
}

// diffMatch 差分识别:逐字段注入互不相同的哨兵值,观察闭包输出是否变化。
func (r *resolver) diffMatch(sel any) *schema.Field {
	sv := reflect.ValueOf(sel)
	var hits []*schema.Field
	for _, f := range r.fields {
		v1, v2 := reflect.New(r.modelType).Elem(), reflect.New(r.modelType).Elem()
		if !applyDistinct(v1, f, 1) || !applyDistinct(v2, f, 2) {
			continue
		}
		o1, ok1 := safeCall(sv, v1.Addr())
		o2, ok2 := safeCall(sv, v2.Addr())
		if ok1 && ok2 && !reflect.DeepEqual(o1.Interface(), o2.Interface()) {
			hits = append(hits, f)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0]
	case 0:
		return nil
	default:
		panic("gorm_max: 字段选择器影响了多个字段,无法唯一识别,建议改用指针写法 return &u.X")
	}
}

// safeCall 调用闭包;内部 panic(如解引用 nil 字段)视为不匹配。
func safeCall(fn, arg reflect.Value) (out reflect.Value, ok bool) {
	defer func() {
		if recover() != nil {
			out, ok = reflect.Value{}, false
		}
	}()
	out = fn.Call([]reflect.Value{arg})[0]
	return out, true
}

func columnOf(f *schema.Field) *Column {
	return &Column{Name: f.DBName, FieldName: f.Name, FieldType: f.FieldType}
}

// pkValue 取主键值,零值返回 false。
func (r *resolver) pkValue(entity any) (any, bool) {
	if r.pk == nil || entity == nil {
		return nil, false
	}
	v := reflect.ValueOf(entity)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil, false
	}
	fv := r.pk.ReflectValueOf(context.Background(), v)
	if !fv.IsValid() || fv.IsZero() {
		return nil, false
	}
	return fv.Interface(), true
}

func applyDistinct(v reflect.Value, f *schema.Field, n int) bool {
	fv := f.ReflectValueOf(context.Background(), v)
	if !fv.IsValid() || !fv.CanSet() {
		return false
	}
	val, ok := distinctOfType(fv.Type(), n)
	if !ok {
		return false
	}
	fv.Set(val)
	return true
}

// distinctOfType 生成某类型第 n 个互不相同的哨兵值。
func distinctOfType(t reflect.Type, n int) (reflect.Value, bool) {
	switch t.Kind() {
	case reflect.String:
		return reflect.ValueOf(fmt.Sprintf("#%d", n)), true
	case reflect.Bool:
		return reflect.ValueOf(n == 1), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return reflect.ValueOf(n).Convert(t), true
	case reflect.Float32, reflect.Float64:
		return reflect.ValueOf(float64(n) + 0.5).Convert(t), true
	case reflect.Ptr:
		p := reflect.New(t.Elem())
		if inner, ok := distinctOfType(t.Elem(), n); ok {
			p.Elem().Set(inner)
		}
		return p, true
	case reflect.Slice:
		return reflect.MakeSlice(t, n, n), true
	case reflect.Map:
		return reflect.MakeMapWithSize(t, n), true
	case reflect.Struct:
		if t == reflect.TypeOf(time.Time{}) {
			return reflect.ValueOf(time.Unix(int64(n), 0).UTC()), true
		}
		for i := 0; i < t.NumField(); i++ { // 用首个可差分的导出字段区分
			if !t.Field(i).IsExported() {
				continue
			}
			if inner, ok := distinctOfType(t.Field(i).Type, n); ok {
				sv := reflect.New(t).Elem()
				sv.Field(i).Set(inner)
				return sv, true
			}
		}
	}
	return reflect.Value{}, false
}
