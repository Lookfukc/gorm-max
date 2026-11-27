# 插件

打开数据库后注册一次,作用于所有会话(实现机制是 gorm Callback,不侵入业务代码)。

## 乐观锁

等价 MP 的 `OptimisticLockerInnerInterceptor`:版本号机制防止并发覆盖。

**1. 实体字段加标签:**

```go
type Product struct {
    ID      int64 `gorm:"primaryKey"`
    Name    string
    Stock   int
    Version int   `gorm:"version"` // ← 乐观锁版本号
}
```

**2. 注册:**

```go
plugin.RegisterOptimisticLock(db)
```

**3. 行为:** 结构体更新(`UpdateByID` / `SaveOrUpdate`)自动变为

```sql
UPDATE `products` SET ..., `version`=旧+1 WHERE `id`=? AND `version`=旧
```

影响行数为 0 → 版本已被别人改过。**版本值 0 视为未启用**(首次插入即从 1 起算)。

注意:乐观锁只对**结构体更新**生效;`Update`(wrapper 更新)不经过版本比对。

## 多租户

等价 MP 的 `TenantLineInnerInterceptor`:tenant_id 自动注入与填充。

**1. 注册(模型带 `tenant_id` 列,可自定义):**

```go
plugin.RegisterTenant(db,
    plugin.WithTenantColumn("tenant_id"),              // 可选,默认 tenant_id
    plugin.WithSkipTables("global_config", "dict"),    // 可选,跳过的表
)
```

**2. 请求链路携带租户标识:**

```go
ctx = plugin.WithTenant(ctx, "t001") // 通常在中间件里从 JWT 取出后注入
```

**3. 行为:**

- SELECT / UPDATE / DELETE → 自动追加 `AND tenant_id = ?`;
- INSERT → 自动填充 `tenant_id` 列;
- ctx 未携带租户的会话按无租户处理(网关侧应保证已注入)。

> 方法签名与配置项见 [API 参考:插件](../api/plugin.md)。
