# CRUD 接口

gorm-max 的 CRUD 由两个泛型类型承担,与 MP 的对应关系:

| MP | gorm-max | 说明 |
|---|---|---|
| `BaseMapper<T>` | `Mapper[T]` | 泛型 CRUD 门面,绑定 `*gorm.DB` 与模型 `T` |
| `IService<T>` / `ServiceImpl` | `Service[T]` | 内嵌 `*Mapper[T]`,追加业务组合方法,**Mapper 全部方法可直接使用** |

```go
m   := gorm_max.NewMapper[User](db)    // 只需 CRUD 门面时
svc := gorm_max.NewService[User](db)   // 常规推荐(含 Query()/NewUpdate() 简写与事务糖)
```

> 本文示例中 `fName` / `fAge` 等是字段选择器闭包的简写变量,如 `fName := func(u *User) any { return &u.Name }`,详见[条件构造器](wrapper.md)。

> MP 中你继承接口;gorm-max 中你**组合**一个实例——无需为每个模型写一层样板代码,注入/测试替换也更容易。

## 方法全景

| 分类 | 方法 |
|---|---|
| 插入 | `Insert` `InsertBatch` |
| 查询 | `SelectByID` `SelectOne` `SelectList` `SelectCount` `SelectPage` `Exists` |
| 更新 | `UpdateByID`(结构体,跳过零值) `Update`(wrapper,map SET) `SaveOrUpdate` `SaveOrUpdateBatch` |
| 删除 | `DeleteByID` `Delete`(有 `DeletedAt` 字段时自动软删) |
| 构造器 | `Query()` / `NewQuery()`(等价 MP 的 `lambdaQuery()`)、`NewUpdate()`(等价 `lambdaUpdate()`) |
| 事务 | `Tx(tx)` `Transaction(ctx, fn)` |
| 逃生口 | `DB(ctx)`(原生 `*gorm.DB`) |

逐方法签名/参数/示例见 [API 参考:Mapper 与 Service](../api/mapper.md)。

## 各分类典型用法

### 插入

```go
u := &User{Name: "tom", Age: 20}
_ = svc.Insert(ctx, u)                        // 自增主键自动回填 u.ID
_ = svc.InsertBatch(ctx, []*User{{...},{...}}) // 默认 500/批,可传 batchSize
```

### 查询

```go
u, err := svc.SelectByID(ctx, 1)              // 不存在 → gorm.ErrRecordNotFound
one, _ := svc.SelectOne(ctx, svc.Query().Eq(fName, "tom"))  // LIMIT 1
list, _ := svc.SelectList(ctx, svc.Query().Gt(fAge, 10))
cnt, _  := svc.SelectCount(ctx, svc.Query().Like(fName, "j"))
ok, _   := svc.Exists(ctx, svc.Query().Eq(fEmail, "a@x.com"))
```

分页见 [分页](page.md);条件构造见 [条件构造器](wrapper.md)。

### 更新:两种语义,按需选择

| 方式 | SET 语义 | 适用 |
|---|---|---|
| `UpdateByID(ctx, u)` | 结构体非零字段 | 常规编辑保存 |
| `svc.Update(ctx, svc.NewUpdate().Set(fAge, 0).Eq(fID, 1))` | 显式 SET 列表 | **需要写零值**、自增表达式、批量条件更新 |

```go
n, _ := svc.Update(ctx, svc.NewUpdate().
    SetItems(q.NickName.SetTo("n1"), q.Age.Incr(1)).   // SET nick_name='n1', age=age+1
    Where(q.Name.Eq("tom")))
```

`SaveOrUpdate`:主键为零 → 插入,非零 → 更新(走结构体更新语义)。

### 删除

```go
n, _ := svc.DeleteByID(ctx, 1)
n, _ = svc.Delete(ctx, svc.Query().Lt(fAge, 0))   // 无 WHERE 会被拒绝(ErrFullTableDelete)
```

模型带 `gorm.DeletedAt` 字段时自动软删(等价 MP `@TableLogic`),查询自动过滤已删数据。

## 约定

1. 所有方法第一个参数是 `context.Context`;
2. 未找到记录统一返回 `gorm.ErrRecordNotFound`(透传 gorm);
3. 无 WHERE 的 `Update`/`Delete` 默认拒绝,`WithAllowFullTable()` 放开——等价 MP 的"防全表更新与删除插件",这里是默认开启的。
