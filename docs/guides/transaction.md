# 事务

两种方式,与 MP 的 `@Transactional` / `TransactionTemplate` 对应关系:

| MP | gorm-max | 说明 |
|---|---|---|
| `@Transactional`(声明式) | `svc.Transaction(ctx, fn)` | 回调式事务糖,自动提交/回滚 |
| 编程式事务 | `svc.Tx(tx)` + gorm 原生 `Begin/Commit` | 手动控制,灵活组合 |

## 推荐:Transaction 事务糖

回调拿到**绑定了事务的新 Service**;返回 nil 提交,返回 error 或 panic 自动回滚;**支持嵌套**(内层自动降级为 SavePoint):

```go
err := svc.Transaction(ctx, func(tx *gorm_max.Service[User]) error {
    if err := tx.Insert(ctx, u); err != nil {
        return err // 返回 error → 整体回滚
    }
    _, err := tx.Update(ctx, tx.NewUpdate().Set(fAge, 1).Eq(fName, "tom"))
    return err
})
```

回调内要用到其他模型时,为每个模型各绑一个事务 Service:

```go
err := svc.Transaction(ctx, func(tx *gorm_max.Service[User]) error {
    orderSvc := gorm_max.NewService[Order](tx.DB(ctx)) // 复用同一事务连接
    return orderSvc.Insert(ctx, o)
})
```

## 手动控制:Tx 绑定

等价 gorm 原生流程,`Tx()` 返回的 Mapper 上所有操作都走该事务:

```go
tx := db.Begin()
defer func() { if r := recover(); r != nil { tx.Rollback() } }()

tm := svc.Tx(tx)
if err := tm.Insert(ctx, u); err != nil {
    tx.Rollback()
    return err
}
tx.Commit()
```

## ⚠️ 最重要的一条

**回调内所有操作必须走 `tx` 参数**(或 `Tx()` 返回的实例)**,混用外层 `svc` 的语句会脱离事务**——这是回调式事务最常见的坑,编译器无法帮你检查。

> 方法签名见 [API 参考:Transaction / Tx](../api/mapper.md)。
