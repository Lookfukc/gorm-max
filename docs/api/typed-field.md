# 类型化字段体系

基于泛型的字段句柄:**列名 + 值类型 V 绑定在一起**,条件/赋值/排序方法的值参数在**编译期**校验——`q.Age.Gt("abc")` 直接编译报错,`q.Name` 是真实 Go 标识符(重构安全),且零运行时解析。

字段句柄两种来源:

1. **生成器产出**(推荐):`gormmaxgen` 为模型生成字段集包(见 [codegen.md](codegen.md));
2. **手写**:用本页构造器手工声明,形态与生成物完全一致。

```go
// 手写示例(模拟生成物形态)
var UserFields = struct {
	ID   gorm_max.Int64Field
	Name gorm_max.StringField
	Age  gorm_max.IntField
}{
	ID:   gorm_max.NewInt64("id"),
	Name: gorm_max.NewString("name"),
	Age:  gorm_max.NewInt("age"),
}

q := UserFields
svc.SelectList(ctx, svc.Query().Where(q.Name.Like("tom"), q.Age.Gt(10)))
```

挂载入口在 wrapper 上:[`Where(conds...)`](query-wrapper.md#where)、[`Order(ords...)`](query-wrapper.md#order)、[`SelectF(fs...)`](query-wrapper.md#selectf)、[`UpdateWrapper.SetItems(items...)`](update-wrapper.md#setitems)。与闭包 API 可混用。

---

## TypedField[V]

```go
type TypedField[V any] struct{ /* 列名 */ }
```

通用字段句柄,全部条件/赋值/排序方法如下。

### 条件方法(返回 Cond)

| 方法 | 签名 | SQL 片段 |
|---|---|---|
| Eq | `Eq(v V) Cond` | `column = ?` |
| Ne | `Ne(v V) Cond` | `column <> ?` |
| Gt | `Gt(v V) Cond` | `column > ?` |
| Ge | `Ge(v V) Cond` | `column >= ?` |
| Lt | `Lt(v V) Cond` | `column < ?` |
| Le | `Le(v V) Cond` | `column <= ?` |
| IsNull | `IsNull() Cond` | `column IS NULL` |
| IsNotNull | `IsNotNull() Cond` | `column IS NOT NULL` |
| In | `In(vals ...V) Cond` | `column IN (?,...)`(空值 panic) |
| NotIn | `NotIn(vals ...V) Cond` | `column NOT IN (?,...)`(空值 panic) |
| Between | `Between(a, b V) Cond` | `column BETWEEN ? AND ?` |
| NotBetween | `NotBetween(a, b V) Cond` | `column NOT BETWEEN ? AND ?` |

```go
q.Age.Gt(10)          // ✓ int
// q.Age.Gt("abc")    // ✗ 编译错误:string 不是 int
q.Name.Eq("tom")      // ✓ string
q.CreatedAt.Gt(someTime)
```

### 赋值方法(返回 SetItem,经 SetItems 挂载)

| 方法 | 签名 | 语义 |
|---|---|---|
| SetTo | `SetTo(v V) SetItem` | `SET column = v` |
| Incr | `Incr(n int64) SetItem` | `SET column = column + n`(列名经方言转义;n ≤ 0 panic;仅整型列) |
| Decr | `Decr(n int64) SetItem` | `SET column = column - n`(防负数在 Where 叠加自身条件) |

```go
svc.Update(ctx, svc.NewUpdate().
	SetItems(q.NickName.SetTo("n1"), q.Age.Incr(1)).
	Where(q.Name.Eq("tom")))
```

### 排序方法(返回 Order,经 Order 挂载)

| 方法 | 签名 | 语义 |
|---|---|---|
| Asc | `Asc() Order` | `ORDER BY column ASC` |
| Desc | `Desc() Order` | `ORDER BY column DESC` |

```go
svc.SelectList(ctx, svc.Query().Order(q.ID.Desc(), q.Name.Asc()))
```

### Column

```go
func (f TypedField[V]) Column() string
```

返回该句柄绑定的 DB 列名(满足 `FieldRef` 接口,用于 SelectF 等)。

---

## 具体字段类型与构造器

具体类型为**定义类型**(非别名),以便携带专属方法(如 StringField 的 Like 系)而不污染其他类型。

| 构造器 | 字段类型 | V | 说明 |
|---|---|---|---|
| `NewString(col)` | `StringField` | `string` | 额外携带 Like 系方法 |
| `NewInt(col)` | `IntField` | `int` | |
| `NewInt8/16/32/64(col)` | `Int8Field` 等 | `int8..64` | |
| `NewUint/8/16/32/64(col)` | `UintField` 等 | `uint..64` | |
| `NewFloat32/64(col)` | `Float32Field/Float64Field` | `float32/64` | |
| `NewBool(col)` | `BoolField` | `bool` | |
| `NewTime(col)` | `TimeField` | `time.Time` | `time.Time`/`gorm.DeletedAt` 均归此类 |
| `NewBytes(col)` | `BytesField` | `[]byte` | |
| `NewAny(col)` | `GenericField` | `any` | 兜底:unnamed 复合类型、serializer JSON 列 |
| `NewTyped[V](col)` | `TypedField[V]` | 任意自定义类型 | 精确到原类型,如 `NewTyped[model.Status]("status")` |

---

## StringField 专属:模糊匹配

```go
func (f StringField) Like(kw string) Cond          // LIKE '%kw%'
func (f StringField) NotLike(kw string) Cond       // NOT LIKE '%kw%'
func (f StringField) LikeLeft(prefix string) Cond  // LIKE 'prefix%'(前缀,可走索引)
func (f StringField) LikeRight(suffix string) Cond // LIKE '%suffix'(后缀)
```

```go
q.Name.Like("tom")   // LIKE '%tom%'
q.Name.LikeLeft("tom") // LIKE 'tom%'
```

> Like 系只在 `StringField` 上存在——对 int 字段调用 `Like` 是编译错误,这是类型化体系的价值之一。

---

## 条件组合子

```go
func Or(conds ...Cond) Cond  // 组内 OR 连接,整体 AND 接入:a AND (b OR c)
func And(conds ...Cond) Cond // 组内 AND 连接(平铺时与直接传入 Where 等价,嵌套时用于表达结构)
func Not(conds ...Cond) Cond // 组内 AND 连接并取反:NOT (a AND b)
```

空组 panic。支持任意深度嵌套:

```go
// name = ? AND (age > ? OR (age < ? AND email IS NOT NULL))
svc.Query().Where(
	q.Name.Eq("a"),
	gorm_max.Or(q.Age.Gt(1), gorm_max.And(q.Age.Lt(100), q.Email.IsNotNull())),
)
```

组合语义说明:

- **顶层平铺**:`Where` 的多个参数彼此 AND 连接;
- **OR 接入的分组**(即 `a OR (b AND c)`):用 wrapper 的 [`OrSub`](query-wrapper.md#orsub) 表达,可与 Where 混用;
- `Or()` 开关后的第一个 `Where` 条件同样以 OR 接入(与闭包 API 语义一致)。

---

## Cond / SetItem / Order

三个公开类型均由字段方法产生、由 wrapper 消费,字段未导出(只能通过 API 构造,保证合法性):

| 类型 | 产生者 | 消费者 | 说明 |
|---|---|---|---|
| `Cond` | 字段条件方法 + `And/Or/Not` | `QueryWrapper.Where` | 条件节点,含嵌套分组与取反 |
| `SetItem` | `SetTo/Incr/Decr` | `UpdateWrapper.SetItems` | 赋值项 |
| `Order` | `Asc/Desc` | `QueryWrapper.Order` | 排序项 |

## FieldRef

```go
type FieldRef interface {
	Column() string
}
```

所有字段句柄的公共接口(`SelectF` 的参数类型);生成器为每个模型产出的字段集结构体,其字段全部满足该接口。
