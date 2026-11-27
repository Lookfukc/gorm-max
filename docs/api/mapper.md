# Mapper[T] 与 Service[T]

`Mapper[T]` 是泛型 CRUD 门面:构造时绑定 `*gorm.DB` 与模型 `T`,内部只存"工厂",每次执行派生独立 session(无链式污染)。`Service[T]` 内嵌 `*Mapper[T]`,追加业务组合方法,**Mapper 的全部方法在 Service 上同样可用**。

## 构造

### NewMapper

```go
func NewMapper[T any](db *gorm.DB, opts ...MapperOption) *Mapper[T]
```

创建绑定到模型 `T` 的 CRUD 门面。

| 参数 | 说明 |
|---|---|
| `db` | 已打开的 `*gorm.DB`(任意方言) |
| `opts` | 可选配置,见下方 MapperOption |

**示例**

```go
import gorm_max "github.com/Lookfukc/gorm-max"

m := gorm_max.NewMapper[User](db)

// 带选项:显式放开全表操作 + 自定义命名策略
m := gorm_max.NewMapper[User](db,
	gorm_max.WithAllowFullTable(),
	gorm_max.WithNamer(schema.NamingStrategy{SingularTable: true}))
```

> 泛型参数 `T` 必须是命名结构体类型;实体一旦使用,resolver 即被缓存(按类型+命名策略),零重复解析成本。

### MapperOption

| 选项 | 功能 |
|---|---|
| `WithAllowFullTable()` | 允许无 WHERE 的全表更新/删除。默认禁止,触发时返回 `ErrFullTableUpdate` / `ErrFullTableDelete` |
| `WithNamer(n schema.Namer)` | 自定义列名/表名命名策略。默认取 `db.Config.NamingStrategy`,再退到 snake_case。字段选择器解析与命名策略联动 |

---

## 写操作

### Insert

```go
func (m *Mapper[T]) Insert(ctx context.Context, entity *T) error
```

插入单条记录。自增主键自动回填到 `entity`;雪花等回调主键同样回填。

```go
u := &User{Name: "tom", Age: 20}
if err := m.Insert(ctx, u); err != nil { /* ... */ }
fmt.Println(u.ID) // 已回填
```

### InsertBatch

```go
func (m *Mapper[T]) InsertBatch(ctx context.Context, entities []*T, batchSize ...int) error
```

批量插入,底层 `CreateInBatches`。空切片直接返回 nil。

| 参数 | 说明 |
|---|---|
| `entities` | 实体指针切片 |
| `batchSize` | 每批条数,可选;默认 500 |

```go
_ = m.InsertBatch(ctx, []*User{{Name: "a"}, {Name: "b"}}, 200)
```

### UpdateByID

```go
func (m *Mapper[T]) UpdateByID(ctx context.Context, entity *T) (int64, error)
```

按主键更新,**跳过零值字段**(与 gorm 结构体更新语义一致;主键不进 SET)。乐观锁插件启用时自动改写版本条件。

**返回**:受影响行数;主键未设置时返回 `ErrPrimaryKeyMissing`。

```go
u, _ := m.SelectByID(ctx, 1)
u.NickName = "tomcat"
n, _ := m.UpdateByID(ctx, u) // UPDATE ... SET nick_name='tomcat' WHERE id=1
```

> 需要把某字段写成零值时,不要用本方法(gorm 跳过零值),改用 [`Update`](#update) + [`Set`](update-wrapper.md#set)。

### Update

```go
func (m *Mapper[T]) Update(ctx context.Context, uw *UpdateWrapper[T]) (int64, error)
```

按条件更新,SET 部分来自 wrapper 的 Set/SetExpr/Incr/Decr/SetItems(以 map 形式交给 gorm,列名经方言转义)。

**返回**:受影响行数。

| 错误 | 触发条件 |
|---|---|
| `ErrNoSetClause` | 没有调用任何 Set 系方法 |
| `ErrFullTableUpdate` | 没有任何 WHERE 条件且未开启 `WithAllowFullTable` |

```go
n, err := m.Update(ctx, m.NewUpdate().
	Set(func(u *User) any { return &u.NickName }, "tomcat").
	Incr(func(u *User) any { return &u.Age }, 1).
	Eq(func(u *User) any { return &u.Name }, "tom"))
```

### DeleteByID

```go
func (m *Mapper[T]) DeleteByID(ctx context.Context, id any) (int64, error)
```

按主键删除。实体含软删字段(`gorm.DeletedAt` / `soft_delete.DeletedAt`)时自动变 UPDATE 软删;否则物理删除。

| 参数 | 说明 |
|---|---|
| `id` | 主键值,int64/uint64/string 等均可 |

```go
n, _ := m.DeleteByID(ctx, 1)
```

### Delete

```go
func (m *Mapper[T]) Delete(ctx context.Context, w *QueryWrapper[T]) (int64, error)
```

按条件删除,软删规则同上。无 WHERE 条件时返回 `ErrFullTableDelete`(除非 `WithAllowFullTable`)。

```go
n, _ := m.Delete(ctx, m.NewQuery().Eq(func(u *User) any { return &u.Status }, 0))
```

---

## 读操作

### SelectByID

```go
func (m *Mapper[T]) SelectByID(ctx context.Context, id any) (*T, error)
```

按主键查一条(底层 gorm `First`,带 `ORDER BY id LIMIT 1`)。

**返回**:未找到时返回 `gorm_max.ErrRecordNotFound`(即 `gorm.ErrRecordNotFound`,可用 `errors.Is` 判断)。

```go
u, err := m.SelectByID(ctx, 1)
if errors.Is(err, gorm_max.ErrRecordNotFound) {
	// 记录不存在
}
```

### SelectOne

```go
func (m *Mapper[T]) SelectOne(ctx context.Context, w *QueryWrapper[T]) (*T, error)
```

按条件查一条,自动 `LIMIT 1`。条件来自 wrapper(可含排序——排序优先于取第一条)。

```go
u, err := m.SelectOne(ctx, m.NewQuery().
	Gt(func(u *User) any { return &u.Age }, 10).
	OrderByDesc(func(u *User) any { return &u.ID }))
```

### SelectList

```go
func (m *Mapper[T]) SelectList(ctx context.Context, w *QueryWrapper[T]) ([]*T, error)
```

按条件查列表,返回实体指针切片。`w` 可为 nil(查全部)。

```go
users, err := m.SelectList(ctx, m.NewQuery().
	Like(func(u *User) any { return &u.Name }, "tom").
	OrderByDesc(func(u *User) any { return &u.ID }))
```

### SelectCount

```go
func (m *Mapper[T]) SelectCount(ctx context.Context, w *QueryWrapper[T]) (int64, error)
```

按条件计数。带 GroupBy 的 wrapper 会导致计数语义变化,不要混用。

### SelectPage

```go
func (m *Mapper[T]) SelectPage(ctx context.Context, w *QueryWrapper[T], p *Page) (*PageResult[T], error)
```

分页查询:先 count(带同样条件)再 limit/offset,一次调用返回完整分页信息。

| 参数 | 说明 |
|---|---|
| `w` | 条件,可为 nil |
| `p` | 分页参数,可为 nil(默认第 1 页、10 条) |

**返回**:`PageResult[T]`,见下文。

```go
page, err := m.SelectPage(ctx,
	m.NewQuery().Gt(func(u *User) any { return &u.Age }, 10),
	&gorm_max.Page{Current: 2, Size: 20})
fmt.Println(page.Total, page.Pages, len(page.Records))
```

> wrapper 上若设置了 `Limit`/`Offset`,会被分页参数**覆盖**。

### Exists

```go
func (m *Mapper[T]) Exists(ctx context.Context, w *QueryWrapper[T]) (bool, error)
```

是否存在匹配记录(内部即 SelectCount > 0)。

```go
ok, err := m.Exists(ctx, m.NewQuery().Eq(func(u *User) any { return &u.Name }, "tom"))
```

---

## 构造器与基础设施

### NewQuery

```go
func (m *Mapper[T]) NewQuery() *QueryWrapper[T]
func NewQuery[T any]() *QueryWrapper[T] // 包级函数,默认 snake_case 命名
```

创建查询条件构造器。**Mapper 方法版**使用 Mapper 的命名策略(推荐);包级版用默认 snake_case。方法详见 [QueryWrapper](query-wrapper.md)。

### NewUpdate

```go
func (m *Mapper[T]) NewUpdate() *UpdateWrapper[T]
func NewUpdate[T any]() *UpdateWrapper[T]
```

创建更新构造器,命名策略同上。方法详见 [UpdateWrapper](update-wrapper.md)。

### Tx

```go
func (m *Mapper[T]) Tx(tx *gorm.DB) *Mapper[T]
```

返回绑定到事务的 Mapper 克隆(resolver/配置共享,无额外开销)。在 `db.Transaction` 回调或手动 `Begin` 之后使用。

```go
err := db.Transaction(func(tx *gorm.DB) error {
	return m.Tx(tx).Insert(ctx, u)
})
```

### Transaction

```go
func (m *Mapper[T]) Transaction(ctx context.Context, fn func(tx *Mapper[T]) error) error
```

事务糖:开启事务并以绑定该事务的 Mapper 执行回调。

- 回调返回 `error` → 自动回滚,错误透传;
- 回调 `panic` → 回滚后继续上抛;
- 支持**嵌套事务**(内部 SavePoint),内层回滚不影响外层;
- ctx 绑定到事务,取消可传导。

```go
err := m.Transaction(ctx, func(tx *gorm_max.Mapper[User]) error {
	if err := tx.Insert(ctx, u); err != nil {
		return err
	}
	_, err := tx.Update(ctx, tx.NewUpdate().Incr(fAge, 1).Eq(fName, "tom"))
	return err
})
```

> **回调内所有操作必须走 tx 参数**,混用外层 m 的语句会脱离事务。

### DB

```go
func (m *Mapper[T]) DB(ctx context.Context) *gorm.DB
```

返回模型作用域的**全新** gorm 会话(已 `Model(new(T))`、带 ctx)。用于 wrapper 覆盖不到的场景:复杂 join、窗口函数、CTE、聚合扫描、Unscoped 等。

```go
var st struct{ Cnt int64 }
_ = m.DB(ctx).Select("COUNT(*) AS cnt").Scan(&st).Error

_ = m.DB(ctx).Unscoped().Delete(&User{}) // 物理删除
```

> 每次调用都是独立会话,**返回值请勿保存复用**。

---

## Service[T]

### NewService

```go
func NewService[T any](db *gorm.DB, opts ...MapperOption) *Service[T]
```

内嵌 `*Mapper[T]` 的业务门面:Mapper 的全部方法直接提升可用,额外提供 Query/SaveOrUpdate/Transaction。

### Query

```go
func (s *Service[T]) Query() *QueryWrapper[T]
```

等价 MP 的 `lambdaQuery()`:链式构造条件后传给 SelectList/SelectPage 等。

```go
users, _ := svc.SelectList(ctx, svc.Query().
	Gt(func(u *User) any { return &u.Age }, 10).
	OrderByDesc(func(u *User) any { return &u.ID }))
```

### SaveOrUpdate

```go
func (s *Service[T]) SaveOrUpdate(ctx context.Context, entity *T) error
```

主键为空 → 插入(回填主键);主键非空 → 按主键更新(不检查存在性)。

```go
u := &User{Name: "tom"}
_ = svc.SaveOrUpdate(ctx, u) // INSERT
u.Age = 21
_ = svc.SaveOrUpdate(ctx, u) // UPDATE ... WHERE id=...
```

### SaveOrUpdateBatch

```go
func (s *Service[T]) SaveOrUpdateBatch(ctx context.Context, entities []*T, batchSize ...int) error
```

批量版:无主键的聚合走 `InsertBatch`,有主键的逐条 `UpdateByID`。

### Transaction(Service 版)

```go
func (s *Service[T]) Transaction(ctx context.Context, fn func(tx *Service[T]) error) error
```

与 Mapper.Transaction 同语义,回调拿到的是绑好事务的 `Service[T]`:

```go
err := svc.Transaction(ctx, func(tx *gorm_max.Service[User]) error {
	return tx.Insert(ctx, u)
})
```

---

## 分页类型

### Page

```go
type Page struct {
	Current int64 // 页码,从 1 开始;小于 1 时按 1 处理
	Size    int64 // 每页条数;0 时 Pages 不计算
}
```

### PageResult[T]

```go
type PageResult[T any] struct {
	Records []*T   // 当前页数据(total 为 0 时为 nil)
	Total   int64  // 总记录数
	Current int64  // 当前页码
	Size    int64  // 每页条数
	Pages   int64  // 总页数 = ceil(Total/Size);Size 为 0 时不计算
}
```

---

## 错误变量

| 变量 | 触发场景 | 处理建议 |
|---|---|---|
| `ErrRecordNotFound` | SelectByID/SelectOne 未找到(即 `gorm.ErrRecordNotFound`) | `errors.Is(err, gorm_max.ErrRecordNotFound)` |
| `ErrFullTableUpdate` | `Update` 无 WHERE 且未开启 WithAllowFullTable | 确认业务是否真要全表更新,再放开选项 |
| `ErrFullTableDelete` | `Delete` 无条件且未开启选项 | 同上 |
| `ErrPrimaryKeyMissing` | `UpdateByID` 实体主键为零值 | 先查出实体再改再存 |
| `ErrNoSetClause` | `Update` 未调用任何 Set 系方法 | 补充 SET |
