# 原子 upsert

唯一键冲突时"插入或更新"在**一条 SQL 内原子完成**,无应用层"先查后插"竞态窗口:

```go
err := svc.InsertOnConflict(ctx, u, gorm_max.ConflictAction{
    Columns:      []string{"name"},                          // 冲突判定列
    UpdateFields: []any{func(x *User) any { return &x.Age }}, // 冲突时只更新这些列
})
err = svc.InsertBatchOnConflict(ctx, users, action)           // 批量版
```

## ConflictAction 策略

| 配置 | 行为 |
|---|---|
| `UpdateAll: true` | 冲突 → 更新全部列(全列覆盖) |
| `UpdateFields: [...]` | 冲突 → 只更新指定列 |
| `Ignore: true` | 冲突 → 忽略本次插入(`INSERT IGNORE` / `ON CONFLICT DO NOTHING`) |
| 三者全空 | 等价 `Ignore` |

互斥配置(同时给 `UpdateAll` 与 `Ignore` 等)返回 `ErrConflictActionInvalid`。

## 方言映射

| 方言 | 生成 |
|---|---|
| MySQL | `INSERT ... ON DUPLICATE KEY UPDATE ...`(按表上唯一键判定) |
| PostgreSQL / SQLite | `INSERT ... ON CONFLICT (columns) DO UPDATE / DO NOTHING` |

## ⚠️ 唯一键陷阱

`UpdateAll: true` 且未指定 `Columns` 时,gorm 默认以**主键**作冲突判定列。如果你的唯一键**不是主键**(如 `name` 上的唯一索引),必须显式指定 `Columns: []string{"name"}`,否则 MySQL 下不会触发冲突分支。

> 方法签名与全部参数见 [API 参考:upsert](../api/upsert.md)。
