# UpdateWrapper[T] 更新构造器

条件更新构造器:SET 部分(本页"赋值方法")+ WHERE 部分(条件方法,与 QueryWrapper 同名同义,只是返回类型为 `*UpdateWrapper[T]` 以保持链式)。

创建:

```go
uw := m.NewUpdate()          // 推荐:与 Mapper 命名策略一致
uw := gorm_max.NewUpdate[User]() // 包级
```

执行:条件构造完交给 [`Mapper.Update`](mapper.md#update):

```go
n, err := m.Update(ctx, m.NewUpdate().
	Set(func(u *User) any { return &u.NickName }, "tomcat").
	Eq(func(u *User) any { return &u.ID }, 1))
```

> 无 SET → `ErrNoSetClause`;无 WHERE → `ErrFullTableUpdate`(除非 `WithAllowFullTable`)。

---

## 赋值方法

### Set

```go
func (w *UpdateWrapper[T]) Set(f Field[T], v any) *UpdateWrapper[T]
```

赋值:`SET column = value`。列名来自字段选择器,值走占位符。

```go
uw.Set(func(u *User) any { return &u.NickName }, "tomcat")
```

### SetExpr

```go
func (w *UpdateWrapper[T]) SetExpr(f Field[T], expr string, args ...any) *UpdateWrapper[T]
```

表达式赋值,expr 为原生 SQL 片段、值走占位符(适合数据库端计算):

```go
uw.SetExpr(func(u *User) any { return &u.Age }, "age + ?", 1)
```

> expr 里的列名是手写的(保留字列需自行注意);优先用 [`Incr`/`Decr`](#incr)(列名自动转义)。

### Incr

```go
func (w *UpdateWrapper[T]) Incr(f Field[T], n int64) *UpdateWrapper[T]
```

自增:`SET column = column + n`。列名来自字段选择器并**在执行时经方言转义**(保留字列安全),无需手写列名。n ≤ 0 panic(自减请用 Decr)。仅整型列。

```go
// 计数器
m.Update(ctx, m.NewUpdate().Incr(fLoginCount, 1).Eq(fID, userID))
```

### Decr

```go
func (w *UpdateWrapper[T]) Decr(f Field[T], n int64) *UpdateWrapper[T]
```

自减:`SET column = column - n`。扣到负数是被允许的;**防负数请叠加自身条件**(条件不满足时 0 行,数据不变):

```go
// 库存式扣减
n, err := m.Update(ctx, m.NewUpdate().
	Decr(fStock, 1).
	Ge(fStock, 1). // stock >= 1 才扣
	Eq(fID, skuID))
// n == 0 表示库存不足
```

### SetItems

```go
func (w *UpdateWrapper[T]) SetItems(items ...SetItem) *UpdateWrapper[T]
```

挂载类型化赋值项(由字段方法 `SetTo/Incr/Decr` 产生,见 [typed-field.md](typed-field.md)),与 Set/SetExpr/Incr/Decr 可混用:

```go
uw.SetItems(q.NickName.SetTo("n1"), q.Age.Incr(1))
```

---

## 条件方法

与 [QueryWrapper](query-wrapper.md) 同名同义,仅返回类型不同(保持 `*UpdateWrapper[T]` 链式):

| 方法 | SQL 片段 |
|---|---|
| `Eq(f, v)` | `column = ?` |
| `Ne(f, v)` | `column <> ?` |
| `Gt(f, v)` / `Ge(f, v)` | `column > ?` / `column >= ?` |
| `Lt(f, v)` / `Le(f, v)` | `column < ?` / `column <= ?` |
| `IsNull(f)` / `IsNotNull(f)` | `column IS NULL` / `IS NOT NULL` |
| `Like(f, kw)` / `NotLike(f, kw)` | `LIKE '%kw%'` / `NOT LIKE '%kw%'` |
| `In(f, vals...)` / `NotIn(f, vals...)` | `IN (...)` / `NOT IN (...)`(空切片 panic) |
| `Between(f, a, b)` / `NotBetween(f, a, b)` | `BETWEEN ? AND ?` / `NOT BETWEEN ? AND ?` |
| `Or()` | 下一个条件改用 OR 连接 |
| `AndSub(build)` / `OrSub(build)` / `NotSub(build)` | 嵌套分组(AND/OR/NOT) |
| `Where(conds...)` | 挂载类型化条件节点(见 [typed-field.md](typed-field.md)) |

```go
// 完整示例:库存扣减 + 复杂条件
n, err := m.Update(ctx, m.NewUpdate().
	Decr(fStock, 1).
	Set(fUpdatedAtFlag, 1).
	Ge(fStock, 1).
	AndSub(func(q *gorm_max.QueryWrapper[User]) *gorm_max.QueryWrapper[User] {
		return q.Eq(fStatus, 1).Or().Eq(fStatus, 2)
	}).
	Eq(fID, skuID))
```

---

## 其他

### Clone

```go
func (w *UpdateWrapper[T]) Clone() *UpdateWrapper[T]
```

深拷贝更新构造器(SET 与 WHERE 全部独立复制),用于复用基础更新模板:

```go
base := m.NewUpdate().Eq(fTenant, "t1")
uw := base.Clone().Set(fName, "x") // base 不受影响
```
