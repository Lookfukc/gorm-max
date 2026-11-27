# 代码生成器

`gormmaxgen` 为模型生成**类型化字段集**,启用[类型化字段](typed-field.md)写法(等价 MP 的代码生成器产出 Lambda 元信息):

```go
q := query.User                                  // 生成产物
users, _ := svc.SelectList(ctx, svc.Query().Where(q.Name.Like("tom"), q.Age.Gt(10)))
```

## 使用

**1. 写生成器入口**(`example/gen/main.go`,顶部 `//go:build ignore` 使其不参与正常构建):

```go
//go:build ignore

package main

import (
    "log"

    "github.com/Lookfukc/gorm-max/example/model"
    "github.com/Lookfukc/gorm-max/gormmaxgen"
)

func main() {
    if err := gormmaxgen.Generate(gormmaxgen.Config{
        Out:    "query/fields_gen.go", // 相对工作目录
        Models: []any{&model.User{}, &model.Order{}},
    }); err != nil {
        log.Fatal(err)
    }
}
```

**2. 模型文件上加指令,触发生成:**

```go
//go:generate go run ./gen/main.go   // 写在 example/main.go 顶部
```

```bash
go generate ./...
```

**3. 产物**(`query/fields_gen.go`):每个模型一个变量组,每个字段一个类型化句柄。

## Config

| 字段 | 说明 |
|---|---|
| `Out` | 输出文件路径 |
| `Package` | 产物包名(如 `query`) |
| `Models` | 模型实例列表(`&model.User{}`) |
| `Namer` | 可选,自定义列名命名策略(默认与运行时同源:反射 gorm schema 解析) |
| `Import` | 可选,产物 import 的模型包路径 |

## 类型映射

模型字段类型 → 字段句柄类型(决定可用方法,如模糊匹配仅 `StringField`):

| Go 类型 | 句柄类型 |
|---|---|
| `string` | `StringField`(含 Like 系) |
| `int` / `int8`...`int64`、`uint` 系 | `IntField` / `Int64Field` 等 |
| `float32` / `float64` | `FloatField` / `Float64Field` |
| `bool` | `BoolField` |
| `time.Time` | `TimeField` |
| 其他 | `GenericField`(条件走 `any` 值) |

> 全部字段类型与构造器见 [API 参考:类型化字段](../api/typed-field.md) · [代码生成](../api/codegen.md)。
