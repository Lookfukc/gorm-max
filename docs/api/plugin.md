# 插件

包路径:`gorm-max/plugin`。对应 MP 的 Interceptor 体系:打开数据库后**注册一次**,作用于该 `*gorm.DB` 的所有会话(包括事务),不注册则零开销。

```go
import "gorm-max/plugin"

plugin.RegisterOptimisticLock(db) // 乐观锁
plugin.RegisterTenant(db)         // 多租户
```

---

## 乐观锁:RegisterOptimisticLock

```go
func RegisterOptimisticLock(db *gorm.DB) error
```

对应 MP 的 `@Version`。

**约定**:实体版本字段加 `gorm:"version"` 标签:

```go
type User struct {
	// ...
	Version int `gorm:"column:version;default:1;comment:乐观锁版本"`
}
```

**行为**(以结构体为目标的更新,即 `UpdateByID` / `SaveOrUpdate`):

```sql
-- 注册前
UPDATE user SET name=?, version=1 WHERE id=1
-- 注册后(实体 version=1)
UPDATE user SET name=?, version=2 WHERE id=1 AND version=1
```

- 实体的版本字段自动 +1 并回填;
- **受影响行数为 0 即版本冲突**,由业务层决定重试或报错;
- 版本值为 0 视为**未启用**(等价 MP 的 version == null)——插入时请显式给 1 或用列默认值;
- wrapper 的 map 更新(`Update` 方法)不参与乐观锁;
- 无 version 字段的模型完全不受影响。

```go
u, _ := m.SelectByID(ctx, 1)
u.Name = "tomcat"
n, _ := m.UpdateByID(ctx, u)
if n == 0 {
	// 版本冲突:重新读取 → 修改 → 重试
}
```

---

## 多租户:RegisterTenant

```go
func RegisterTenant(db *gorm.DB, opts ...TenantOption) error
```

对应 MP 的 `TenantLineInnerInterceptor`。**约定**:模型具有租户列(默认列名 `tenant_id`)。

行为(ctx 携带租户时):

| 语句 | 注入内容 |
|---|---|
| 查询(含 SelectPage 的 count) | 自动追加 `WHERE tenant_id = ?` |
| 更新 / 删除 | 自动追加 `WHERE tenant_id = ?` |
| 插入 | 实体租户列为**零值**时自动填充(显式赋值优先) |

安全规则:

- ctx **无租户时不注入**——便于运维/后台任务显式操作(数据一致由调用方负责);
- 模型没有租户列的表自动跳过;
- `SkipTables` 显式跳过(如租户表自身、全局字典表);
- 业务 wrapper 里已手写租户条件时会出现重复条件(不影响正确性,只影响可读性)。

```go
db, _ := gorm.Open(mysql.Open(dsn))
_ = plugin.RegisterTenant(db) // 默认列 tenant_id

ctx := plugin.WithTenant(context.Background(), "t001")
m.SelectList(ctx, m.NewQuery()...) // WHERE tenant_id = 't001' 自动追加
m.Insert(ctx, &Order{Amount: 1})   // tenant_id 自动填充 't001'
m.Insert(ctx, &Order{TenantID: "t9"}) // 显式赋值优先,不覆盖
```

### TenantOption

```go
func WithTenantColumn(col string) TenantOption // 覆盖租户列名
func WithSkipTables(tables ...string) TenantOption // 追加忽略的表(按表名)
```

```go
// 租户列叫 org_id,并跳过 user 表
_ = plugin.RegisterTenant(db,
	plugin.WithTenantColumn("org_id"),
	plugin.WithSkipTables("user"))
```

### WithTenant

```go
func WithTenant(ctx context.Context, tenant string) context.Context
```

把租户标识放入 ctx。Mapper/Service 的所有方法都接收 ctx,插件从 `stmt.Context` 读取。ctx 无租户(或空字符串)时不注入。

```go
ctx := plugin.WithTenant(ctx, "t001")
svc.SelectList(ctx, ...) // 自动隔离
```

---

## 与其他能力的关系

- 插件通过 gorm callback 实现,对 `Mapper` / `Service` / `Tx` 事务内的语句**全部生效**(事务会话继承回调);
- 乐观锁 + 多租户可在同一实体上叠加(更新时同时带 `version=?` 与 `tenant_id=?` 条件);
- `Mapper.DB(ctx)` 原生会话同样被插件覆盖(callback 在方言层生效,与 API 无关)。
