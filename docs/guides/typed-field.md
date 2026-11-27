# 类型化字段

类型化字段是闭包条件构造器的升级形态,等价 MP 从 `QueryWrapper` 换到 `LambdaQueryWrapper` 的关系:**字段名与值类型都编译期检查,运行时零解析**。

| 对照 | 闭包式 | 类型化字段 |
|---|---|---|
| 字段名检查 | ✅ 编译期 | ✅ 编译期 |
| 值类型检查 | ❌ 运行期 | ✅ **编译期**(`q.Age.Gt("abc")` 编不过) |
| 需要生成代码 | 否 | 是(`go generate`) |
| 写法长度 | 长 | 短 |

## 三步启用

```go
// 1. 写生成器(gen/main.go,顶部 //go:build ignore)
gormmaxgen.Generate(gormmaxgen.Config{
    Out:    "query/fields_gen.go",
    Models: []any{&model.User{}, &model.Order{}},
})

// 2. 生成
go generate ./...

// 3. 使用
q := query.User
users, _ := svc.SelectList(ctx, svc.Query().Where(q.Name.Like("tom"), q.Age.Gt(10)))
```

字段句柄按模型字段类型落地为 `StringField` / `IntField` / `Int64Field` / `TimeField` 等(见 [代码生成器](codegen.md) 的类型映射表),泛型兜底 `GenericField`。

## 三类产物

**条件(Cond)** → 经 `Where(conds...)` 挂载:

```go
q.Age.Gt(10)          // age > 10
q.Name.Like("tom")    // name LIKE '%tom%'(仅 StringField 有模糊系)
q.Age.Between(18, 30) // age BETWEEN 18 AND 30
```

**赋值(SetItem)** → 经 `SetItems(items...)` 挂载到 UpdateWrapper:

```go
svc.Update(ctx, svc.NewUpdate().
    SetItems(q.NickName.SetTo("n1"), q.Age.Incr(1)).   // SET nick_name='n1', age=age+1
    Where(q.Name.Eq("tom")))
```

**排序(Order)** → 经 `Order(orders...)` 挂载:

```go
svc.Query().Order(q.ID.Desc(), q.Age.Asc())
```

## 组合子

等价 MP 的 `and()` / `or()` / `not()` 嵌套 lambda:

```go
gorm_max.And(q.Age.Gt(10), q.Name.Like("t"))  // AND 组
gorm_max.Or(q.Age.Lt(5), q.ID.Gt(100))        // OR 组,整体 AND 接入
gorm_max.Not(q.Email.IsNull())                // NOT (...)
```

## 混用

类型化条件与闭包条件可在同一 wrapper 上混用(渐进迁移友好):

```go
svc.Query().
    Like(func(u *User) any { return &u.Name }, "tom").  // 闭包
    Where(q.Age.Gt(10))                                  // 类型化
```

> 逐方法签名/参数/示例:[类型化字段参考](../api/typed-field.md) · [生成器配置](../api/codegen.md)
