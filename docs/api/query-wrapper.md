# QueryWrapper[T] 条件构造器

查询条件构造器,对应 MP 的 QueryWrapper。两种字段选择器:

- **闭包选择器** `Field[T]`:`type Field[T any] func(*T) any` —— 推荐 `return &u.Name` 指针写法(识别 O(1)、热路径首选);`return u.Name` 值写法亦可(逐字段差分识别,O(字段数))。字段名是真实 Go 语法,**写错编译不过**。
- **类型化字段句柄**(见 [typed-field.md](typed-field.md)):经生成器产出,值类型编译期校验。

创建方式:

```go
m := gorm_max.NewMapper[User](db)

w := m.NewQuery()          // 推荐:与 Mapper 命名策略一致
w := gorm_max.NewQuery[User]() // 包级:默认 snake_case
```

所有条件方法返回 `*QueryWrapper[T]` 自身,支持链式。**wrapper 实例非并发安全**,每次请求新建;`Clone()` 后的副本可独立使用。

> 列名安全:字段经 gorm schema 解析(白名单)+ 方言转义(反引号/双引号);有 JOIN 时全部列引用自动带表名限定,防同名列歧义。条件值一律走占位符。

---

## 基础比较条件

### Eq

```go
func (w *QueryWrapper[T]) Eq(f Field[T], v any) *QueryWrapper[T]
```

等于:`column = ?`。

```go
m.NewQuery().Eq(func(u *User) any { return &u.Name }, "tom")
```

### Ne

```go
func (w *QueryWrapper[T]) Ne(f Field[T], v any) *QueryWrapper[T]
```

不等于:`column <> ?`。

### Gt / Ge

```go
func (w *QueryWrapper[T]) Gt(f Field[T], v any) *QueryWrapper[T] // column > ?
func (w *QueryWrapper[T]) Ge(f Field[T], v any) *QueryWrapper[T] // column >= ?
```

### Lt / Le

```go
func (w *QueryWrapper[T]) Lt(f Field[T], v any) *QueryWrapper[T] // column < ?
func (w *QueryWrapper[T]) Le(f Field[T], v any) *QueryWrapper[T] // column <= ?
```

### IsNull / IsNotNull

```go
func (w *QueryWrapper[T]) IsNull(f Field[T]) *QueryWrapper[T]    // column IS NULL
func (w *QueryWrapper[T]) IsNotNull(f Field[T]) *QueryWrapper[T] // column IS NOT NULL
```

无值参数。

```go
m.NewQuery().IsNull(func(u *User) any { return &u.Email })
```

---

## 模糊匹配

### Like

```go
func (w *QueryWrapper[T]) Like(f Field[T], kw string) *QueryWrapper[T]
```

包含匹配:`column LIKE '%kw%'`,关键字自动包百分号。

```go
m.NewQuery().Like(func(u *User) any { return &u.Name }, "tom") // LIKE '%tom%'
```

### NotLike

```go
func (w *QueryWrapper[T]) NotLike(f Field[T], kw string) *QueryWrapper[T]
```

排除匹配:`column NOT LIKE '%kw%'`。

### LikeLeft

```go
func (w *QueryWrapper[T]) LikeLeft(f Field[T], prefix string) *QueryWrapper[T]
```

前缀匹配:`column LIKE 'prefix%'`(可走索引)。

```go
m.NewQuery().LikeLeft(func(u *User) any { return &u.Name }, "tom") // LIKE 'tom%'
```

### LikeRight

```go
func (w *QueryWrapper[T]) LikeRight(f Field[T], suffix string) *QueryWrapper[T]
```

后缀匹配:`column LIKE '%suffix'`。

---

## 集合与范围

### In

```go
func (w *QueryWrapper[T]) In(f Field[T], vals ...any) *QueryWrapper[T]
```

集合匹配:`column IN (?, ?, ...)`。**空切片 panic**(避免生成非法 SQL),调用方需自行判空。

```go
m.NewQuery().In(func(u *User) any { return &u.ID }, 1, 2, 3)
```

### NotIn

```go
func (w *QueryWrapper[T]) NotIn(f Field[T], vals ...any) *QueryWrapper[T]
```

排除集合:`column NOT IN (...)`,空切片同样 panic。

### Between / NotBetween

```go
func (w *QueryWrapper[T]) Between(f Field[T], a, b any) *QueryWrapper[T]    // column BETWEEN ? AND ?
func (w *QueryWrapper[T]) NotBetween(f Field[T], a, b any) *QueryWrapper[T] // column NOT BETWEEN ? AND ?
```

闭包区间匹配。

```go
m.NewQuery().Between(func(u *User) any { return &u.Age }, 18, 30)
```

---

## 连接与嵌套分组

### Or

```go
func (w *QueryWrapper[T]) Or() *QueryWrapper[T]
```

**开关方法**:下一个条件改用 OR 连接(对应 MP 的 `.or()`)。

```go
// name = ? OR age > ?
m.NewQuery().
	Eq(func(u *User) any { return &u.Name }, "tom").
	Or().
	Gt(func(u *User) any { return &u.Age }, 60)
```

### AndSub

```go
func (w *QueryWrapper[T]) AndSub(build func(*QueryWrapper[T]) *QueryWrapper[T]) *QueryWrapper[T]
```

嵌套 AND 分组,组内条件默认 AND 连接(组内可用 `Or()` 切换)。空分组自动忽略。

```go
// name LIKE ? AND (age < 10 OR age > 25)
m.NewQuery().
	Like(func(u *User) any { return &u.Name }, "j").
	AndSub(func(q *gorm_max.QueryWrapper[User]) *gorm_max.QueryWrapper[User] {
		return q.Lt(func(u *User) any { return &u.Age }, 10).
			Or().
			Gt(func(u *User) any { return &u.Age }, 25)
	})
```

### OrSub

```go
func (w *QueryWrapper[T]) OrSub(build func(*QueryWrapper[T]) *QueryWrapper[T]) *QueryWrapper[T]
```

嵌套 OR 分组:分组与前面的条件以 **OR** 连接(a OR (b AND c))。

```go
// vip = 1 OR (age > 18 AND enabled = 1)
m.NewQuery().
	Eq(func(u *User) any { return &u.Vip }, 1).
	OrSub(func(q *gorm_max.QueryWrapper[User]) *gorm_max.QueryWrapper[User] {
		return q.Gt(func(u *User) any { return &u.Age }, 18).
			Eq(func(u *User) any { return &u.Enabled }, 1)
	})
```

### NotSub

```go
func (w *QueryWrapper[T]) NotSub(build func(*QueryWrapper[T]) *QueryWrapper[T]) *QueryWrapper[T]
```

嵌套 NOT 分组:`NOT (组内条件,默认 AND 连接)`。支持 `Or().NotSub(...)` 产生 `OR NOT (...)`。

```go
// name LIKE ? AND NOT (status = 0 AND age > 60)
m.NewQuery().
	Like(func(u *User) any { return &u.Name }, "tom").
	NotSub(func(q *gorm_max.QueryWrapper[User]) *gorm_max.QueryWrapper[User] {
		return q.Eq(func(u *User) any { return &u.Status }, 0).
			Gt(func(u *User) any { return &u.Age }, 60)
	})
```

---

## 投影与排序

### Select

```go
func (w *QueryWrapper[T]) Select(fs ...Field[T]) *QueryWrapper[T]
```

指定查询列(字段选择器),不调用则默认所有列。列名经方言转义;有 JOIN 时自动带表名限定。

```go
m.SelectList(ctx, m.NewQuery().
	Select(func(u *User) any { return &u.Name }, func(u *User) any { return &u.Age }))
```

### SelectF

```go
func (w *QueryWrapper[T]) SelectF(fs ...FieldRef) *QueryWrapper[T]
```

类型化字段版投影(见 [typed-field.md](typed-field.md)):

```go
w.SelectF(q.Name, q.Age)
```

### OrderByAsc / OrderByDesc

```go
func (w *QueryWrapper[T]) OrderByAsc(f Field[T]) *QueryWrapper[T]
func (w *QueryWrapper[T]) OrderByDesc(f Field[T]) *QueryWrapper[T]
```

升序/降序排序,可多次调用叠加(按调用顺序)。

```go
m.NewQuery().
	OrderByDesc(func(u *User) any { return &u.CreatedAt }).
	OrderByAsc(func(u *User) any { return &u.ID })
```

### Order

```go
func (w *QueryWrapper[T]) Order(ords ...Order) *QueryWrapper[T]
```

类型化字段版排序,`Order` 由字段方法 `Asc()/Desc()` 产生:

```go
w.Order(q.ID.Desc(), q.Name.Asc())
```

---

## 分组聚合

### GroupBy

```go
func (w *QueryWrapper[T]) GroupBy(fs ...Field[T]) *QueryWrapper[T]
```

分组(可多列)。配合 [`Having`](#having) 做聚合过滤。

```go
// SELECT * FROM users GROUP BY name HAVING COUNT(*) > 1
m.SelectList(ctx, m.NewQuery().
	GroupBy(func(u *User) any { return &u.Name }).
	Having("COUNT(*) > ?", 1))
```

> **注意**:带 GroupBy 的 wrapper 不要用于 `SelectPage`/`SelectCount`——分组下的计数语义会变。

### Having

```go
func (w *QueryWrapper[T]) Having(sql string, args ...any) *QueryWrapper[T]
```

手写 HAVING 片段(聚合表达式无法用字段选择器表达,对应 MP 的 `having(sql, params)`)。sql 为开发者提供的片段;**值一律走占位符**。多个片段自动 `AND` 连接并各自加括号。

```go
w.Having("COUNT(*) > ?", 1).Having("SUM(age) > ?", 10)
// → HAVING (COUNT(*) > 1) AND (SUM(age) > 10)
```

---

## JOIN 与去重

### Joins

```go
func (w *QueryWrapper[T]) Joins(joinSQL string, args ...any) *QueryWrapper[T]
```

原生 JOIN 片段透传,值走占位符,可多次调用叠加;传**关系字段名**时行为同 gorm 的关联 Join。空片段 panic。

```go
// 片段 + 内联条件
m.SelectList(ctx, m.NewQuery().
	Joins("INNER JOIN profiles p ON p.id = users.profile_id AND p.city = ?", "北京"))

// 关联 Join(同 gorm)
m.SelectList(ctx, m.NewQuery().Joins("Profile"))
```

> 关联表的过滤条件建议内联在 JOIN 片段里,或用 [`Apply`](#apply) 追加。**有 JOIN 后,wrapper 的所有列引用自动带表名限定**(`users.name`),避免多表同名列歧义。

### Distinct

```go
func (w *QueryWrapper[T]) Distinct(fs ...Field[T]) *QueryWrapper[T]
```

去重。无参:`SELECT DISTINCT *`;有参:`SELECT DISTINCT 指定列`。与 `Select` 同用且有参时,列以 Distinct 为准。

```go
m.SelectList(ctx, m.NewQuery().Distinct(func(u *User) any { return &u.Name }))
```

> 配合 `SelectPage`/`SelectCount` 时计数语义以 gorm 行为为准,慎用(同 GroupBy)。

---

## 关联预载

### Preload

```go
func (w *QueryWrapper[T]) Preload(assoc string, args ...any) *QueryWrapper[T]
```

关联预载(透传 gorm)。`assoc` 是模型的**关系字段名(Go 字段名,非列名)**,支持嵌套与条件:

| 参数 | 说明 |
|---|---|
| `assoc` | 关系字段名,如 `"Profile"`、`"Orders.Items"`(嵌套) |
| `args` | 可选条件,透传给 gorm 的预载查询 |

```go
m.SelectList(ctx, m.NewQuery().
	Preload("Profile").                              // 全量预载
	Preload("Orders", "state <> ?", "cancelled"))    // 条件预载
```

> 同一关联多次调用时,后者的条件**覆盖**前者(gorm 内部是 map)。空关联名 panic。

---

## 行数截断

### Limit

```go
func (w *QueryWrapper[T]) Limit(n int64) *QueryWrapper[T]
```

返回行数上限,独立于 SelectPage 的轻量截断。n 为负数 panic;0 合法(返回空)。与 SelectPage 同用时被分页参数覆盖。

```go
// 取最新 10 条
m.SelectList(ctx, m.NewQuery().OrderByDesc(fID).Limit(10))
```

### Offset

```go
func (w *QueryWrapper[T]) Offset(n int64) *QueryWrapper[T]
```

跳过的行数,一般与 Limit 同用(MySQL/SQLite 语法要求 OFFSET 跟在 LIMIT 后)。负数 panic。

```go
m.SelectList(ctx, m.NewQuery().OrderByAsc(fID).Limit(2).Offset(2))
```

---

## 行锁

### ForUpdate

```go
func (w *QueryWrapper[T]) ForUpdate(opts ...LockOption) *QueryWrapper[T]
```

行级排他锁:`SELECT ... FOR UPDATE`。**必须在事务内使用**才有意义。

| LockOption | 值 | 语义 |
|---|---|---|
| `LockNoWait` | `NOWAIT` | 锁冲突不等待,立即报错 |
| `LockSkipLocked` | `SKIP LOCKED` | 跳过已被其他事务锁定的行 |

修饰项可组合(`ForUpdate(LockNoWait, LockSkipLocked)`)。

```go
_ = db.Transaction(func(tx *gorm.DB) error {
	// 锁定待扣减的行
	users, err := m.Tx(tx).SelectList(ctx, m.NewQuery().
		Eq(func(u *User) any { return &u.Status }, 1).
		ForUpdate())
	// ... 扣减 ...
	return err
})
```

> MySQL/PG/SQL Server/Oracle 支持;SQLite 不支持(方言按其设计忽略该子句)。不适用于带 GroupBy 的聚合查询;配合 SelectPage 时 count 语句也会带锁。

### ForShare

```go
func (w *QueryWrapper[T]) ForShare(opts ...LockOption) *QueryWrapper[T]
```

行级共享锁:`SELECT ... FOR SHARE`,语义同 ForUpdate,锁模式为共享。

---

## 类型化条件接入口

### Where

```go
func (w *QueryWrapper[T]) Where(conds ...Cond) *QueryWrapper[T]
```

挂载公开条件节点([Cond/And/Or/Not](typed-field.md)),顶层平铺彼此 AND,分组用组合子表达:

```go
q := query.User
w := m.NewQuery().Where(
	q.Name.Like("tom"),
	q.Age.Gt(10),
	gorm_max.Or(q.Age.Lt(5), q.ID.Gt(100)), // a AND (b OR c)
	gorm_max.Not(q.Email.IsNull()),          // AND NOT (email IS NULL)
)
```

与闭包方法可混用;`Or()` 开关后的第一个 Where 条件同样以 OR 接入。

> OR 接入的分组(即 a OR (b AND c))继续用 [`OrSub`](#orsub) 表达,可与 Where 混用。

---

## 其他

### Clone

```go
func (w *QueryWrapper[T]) Clone() *QueryWrapper[T]
```

**深拷贝** wrapper(条件、嵌套分组、投影、排序、预载、JOIN、锁、截断全部独立复制),用于复用基础条件:

```go
base := m.NewQuery().Eq(fStatus, 1).OrderByAsc(fID)
w1 := base.Clone().Like(fName, "tom") // base 不受影响
w2 := base.Clone().Lt(fAge, 5)        // 各自独立
```

### Apply

```go
func (w *QueryWrapper[T]) Apply(fn func(*gorm.DB) *gorm.DB) *QueryWrapper[T]
```

直接操作 `*gorm.DB` 的逃逸口,处理 wrapper 覆盖不到的场景。可多次调用,按顺序执行,在条件/投影/排序/锁之后应用。

```go
w := m.NewQuery().Eq(fStatus, 1).
	Apply(func(db *gorm.DB) *gorm.DB {
		return db.Session(&gorm.Session{}) // 任意 gorm 能力
	})
```
