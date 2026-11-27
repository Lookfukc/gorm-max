# 分页

一次调用完成 count + limit/offset,直接返回 MP 风格的分页结果:

```go
page, err := svc.SelectPage(ctx,
    svc.Query().Gt(func(u *User) any { return &u.Age }, 0),  // 查询条件
    &gorm_max.Page{Current: 1, Size: 10})                    // Current 从 1 起
if err != nil { /* ... */ }

page.Total    // int64 总条数
page.Pages    // 总页数
page.Records  // []*User 当前页数据
page.Current / page.Size
```

## 类型

| 类型 | 字段 | 说明 |
|---|---|---|
| `Page` | `Current`、`Size` | 入参;`Current` 从 1 起 |
| `PageResult[T]` | `Records`、`Total`、`Current`、`Size`、`Pages` | 返回值,泛型记录切片 |

## 生成的 SQL

两次查询,条件完全一致:

```sql
SELECT count(*) FROM `users` WHERE `age` > 0 AND `users`.`deleted_at` IS NULL
SELECT * FROM `users` WHERE `age` > 0 AND `users`.`deleted_at` IS NULL LIMIT 10
```

## 注意事项

1. **带 `GroupBy` / `Distinct` 的 wrapper 不建议用于 `SelectPage`**——分组/去重下的 `count(*)` 语义与"总条数"不一致,需要自行用 `DB(ctx)` 写聚合查询;
2. 排序建议显式给出,否则分页结果顺序不保证稳定;
3. `Size` 为 0 时按 gorm 默认处理(不截断),请显式传值。

> 方法签名与参数见 [API 参考:SelectPage](../api/mapper.md)。
