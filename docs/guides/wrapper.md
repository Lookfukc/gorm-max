# 条件构造器

gorm-max 提供两套条件构造器,与 MP 的 Wrapper 家族一一对应:

| MP | gorm-max | 特点 |
|---|---|---|
| `QueryWrapper`(字符串列名) | **闭包式** `QueryWrapper[T]`(字段选择器闭包) | 字段名编译期检查;零依赖,无需生成代码 |
| `LambdaQueryWrapper` | **类型化字段**(配合 [代码生成](codegen.md)) | 字段名+**值类型**双重编译期检查,代码更短 |
| `UpdateWrapper` | `UpdateWrapper[T]` | SET 子句 + 与 QueryWrapper 同款条件方法 |

两套 API 可在**同一个 wrapper 上混用**;更新用 `svc.NewUpdate()`,查询用 `svc.Query()`。

## 闭包式(默认,无需生成代码)

字段选择器是闭包,**字段名写错/重构改名 → 编译不过**。推荐指针写法 `return &u.Name`(识别 O(1)):

```go
users, _ := svc.SelectList(ctx, svc.Query().
    Like(func(u *User) any { return &u.Name }, "tom").   // name LIKE '%tom%'
    Gt(func(u *User) any { return &u.Age }, 10).         // AND age > 10
    In(func(u *User) any { return &u.Age }, 1, 2, 3).    // AND age IN (1,2,3)
    OrderByDesc(func(u *User) any { return &u.ID }))     // ORDER BY id DESC
// SELECT * FROM `users` WHERE (`name` LIKE '%tom%' AND `age` > 10 AND `age` IN (1,2,3)) ...
```

### 条件方法速览

| 分类 | 方法 |
|---|---|
| 比较 | `Eq` `Ne` `Gt` `Ge` `Lt` `Le` |
| 模糊 | `Like`(`%v%`) `LikeLeft`(`%v`) `LikeRight`(`v%`) `NotLike` |
| 集合/范围 | `In` `NotIn` `Between` `NotBetween` |
| 空值 | `IsNull` `IsNotNull` |
| 连接/嵌套 | `Or` `AndSub` `OrSub` `NotSub` |
| 投影/排序 | `Select` `SelectF` `OrderByAsc` `OrderByDesc` `Order` |
| 分组聚合 | `GroupBy` `Having`(原生 SQL 片段) |
| JOIN/去重/预载 | `Joins` `Distinct` `Preload` |
| 截断/行锁 | `Limit` `Offset` `ForUpdate` `ForShare` |

### OR 与嵌套分组(重点)

默认条件间用 AND 连接;`.Or()` 使**下一个条件**用 OR 连接:

```go
// name LIKE '%j%' OR age > 25
svc.Query().Like(fName, "j").Or().Gt(fAge, 25)
```

嵌套分组用 `AndSub` / `OrSub` / `NotSub`(等价 MP 的 `and(i -> ...)` / `or(i -> ...)` / `not(i -> ...)`):

```go
// name LIKE '%j%' AND (age < 10 OR age > 25)
w := svc.Query().
    Like(func(u *User) any { return &u.Name }, "j").
    AndSub(func(q *gorm_max.QueryWrapper[User]) *gorm_max.QueryWrapper[User] {
        return q.Lt(func(u *User) any { return &u.Age }, 10).
            Or().
            Gt(func(u *User) any { return &u.Age }, 25)
    })
```

## 类型化字段(等价 Lambda 版,推荐正式项目使用)

先[生成字段集](codegen.md),之后值类型也获得编译期校验——`q.Age.Gt("abc")` 直接编译报错:

```go
q := query.User
users, _ := svc.SelectList(ctx, svc.Query().
    Where(q.Name.Like("tom"), q.Age.Gt(10),
        gorm_max.Or(q.Age.Lt(5), q.ID.Gt(100)),   // 组内 OR,整体 AND 接入
        gorm_max.Not(q.Email.IsNull())).          // NOT (...)
    Order(q.ID.Desc()).
    SelectF(q.Name, q.Age))
```

详见 [类型化字段](typed-field.md)。

## 使用提示

1. **wrapper 非并发安全**:每次查询新建,不要跨 goroutine 复用;需要分叉时 `Clone()` 后各自独立;
2. 条件全空 = 查全表(与 MP 一致);无 WHERE 的 Update/Delete 会被[安全防呆](crud.md#约定)拒绝;
3. 列名全部经 schema 白名单 + 方言转义,值全走占位符——不存在列名注入面;
4. 复杂查询不必硬套 wrapper:`Apply(fn)` 可在链上直接操作 `*gorm.DB`,或 `DB(ctx)` 拿原生会话。

> 逐方法签名/参数/示例:[QueryWrapper 参考](../api/query-wrapper.md) · [UpdateWrapper 参考](../api/update-wrapper.md)
