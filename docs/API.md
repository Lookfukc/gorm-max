# gorm-max API 参考

> 包名 `gorm_max`(模块路径 `github.com/Lookfukc/gorm-max`),推荐导入写法:`import gorm_max "github.com/Lookfukc/gorm-max"`
>
> 所有涉及查询的方法第一个参数都是 `context.Context`;未找到记录统一返回 `gorm.ErrRecordNotFound`。

**入门请先读 [快速开始](getting-started.md)**;教学式讲解见上表"指南"层,本页及"参考"层各页为逐方法速查。

## 文档目录

| 文档 | 层 | 内容 |
|---|---|---|
| [快速开始](getting-started.md) | 指南 | 一条龙可复现教程(依赖→配置→编码→验证) |
| [CRUD 接口](guides/crud.md) | 指南 | Mapper/Service 全景、更新语义、软删、安全防呆 |
| [条件构造器](guides/wrapper.md) | 指南 | 两套 Wrapper 体系、OR/嵌套分组、投影/JOIN/行锁 |
| [类型化字段](guides/typed-field.md) | 指南 | 值类型编译期校验的 Lambda 式写法 |
| [分页](guides/page.md) | 指南 | Page / PageResult |
| [事务](guides/transaction.md) | 指南 | 事务糖与手动控制 |
| [原子 upsert](guides/upsert.md) | 指南 | ConflictAction 策略与方言差异 |
| [插件](guides/plugin.md) | 指南 | 乐观锁、多租户 |
| [代码生成器](guides/codegen.md) | 指南 | gormmaxgen 工作流与类型映射 |
| [Mapper 与 Service](api/mapper.md) | 参考 | 泛型 CRUD 门面、事务、分页类型、错误变量、构造选项(逐方法) |
| [QueryWrapper 条件构造器](api/query-wrapper.md) | 参考 | 全部查询条件、嵌套分组、投影/排序/分组/去重、预载/JOIN、行锁、截断(逐方法) |
| [UpdateWrapper 更新构造器](api/update-wrapper.md) | 参考 | Set/SetExpr/Incr/Decr、SetItems、条件方法(逐方法) |
| [类型化字段体系](api/typed-field.md) | 参考 | TypedField、具体字段类型、Cond/And/Or/Not、SetItem/Order |
| [原子 upsert](api/upsert.md) | 参考 | InsertOnConflict / InsertBatchOnConflict / ConflictAction |
| [插件](api/plugin.md) | 参考 | 乐观锁、多租户 |
| [代码生成器](api/codegen.md) | 参考 | gormmaxgen.Config / Generate / 类型映射 |

入门示例与设计说明见 [README](../README.md);接入现有项目的步骤见 [接入手册](INTEGRATION.md)。

---

## 全部公开 API 速查

### Mapper[T](api/mapper.md)

| 方法 | 签名摘要 | 说明 |
|---|---|---|
| NewMapper | `NewMapper[T](db, opts...)` | 构造泛型 CRUD 门面 |
| Insert | `Insert(ctx, entity) error` | 插入单条,主键回填 |
| InsertBatch | `InsertBatch(ctx, entities, batchSize...)` | 批量插入(默认 500/批) |
| SelectByID | `SelectByID(ctx, id) (*T, error)` | 按主键查一条 |
| SelectOne | `SelectOne(ctx, w) (*T, error)` | 按条件查一条(LIMIT 1) |
| SelectList | `SelectList(ctx, w) ([]*T, error)` | 按条件查列表 |
| SelectCount | `SelectCount(ctx, w) (int64, error)` | 按条件计数 |
| SelectPage | `SelectPage(ctx, w, p) (*PageResult[T], error)` | 分页查询 |
| Exists | `Exists(ctx, w) (bool, error)` | 是否存在匹配记录 |
| UpdateByID | `UpdateByID(ctx, entity) (int64, error)` | 按主键更新(跳过零值) |
| Update | `Update(ctx, uw) (int64, error)` | 按条件更新(map SET) |
| DeleteByID | `DeleteByID(ctx, id) (int64, error)` | 按主键删除(软删自动) |
| Delete | `Delete(ctx, w) (int64, error)` | 按条件删除(软删自动) |
| NewQuery | `NewQuery() *QueryWrapper[T]` | 创建查询构造器 |
| NewUpdate | `NewUpdate() *UpdateWrapper[T]` | 创建更新构造器 |
| Tx | `Tx(tx) *Mapper[T]` | 绑定事务 |
| Transaction | `Transaction(ctx, fn)` | 事务糖(嵌套/自动回滚) |
| DB | `DB(ctx) *gorm.DB` | 原生 gorm 逃生口 |
| WithAllowFullTable | 选项 | 允许无 WHERE 全表更新/删除 |
| WithNamer | 选项 | 自定义列名命名策略 |

### Service[T](api/mapper.md)

| 方法 | 签名摘要 | 说明 |
|---|---|---|
| NewService | `NewService[T](db, opts...)` | 构造(内嵌 Mapper,全部方法提升) |
| Query | `Query() *QueryWrapper[T]` | 等价 MP 的 lambdaQuery() |
| SaveOrUpdate | `SaveOrUpdate(ctx, entity)` | 主键空→插入,有→更新 |
| SaveOrUpdateBatch | `SaveOrUpdateBatch(ctx, entities, batchSize...)` | 批量版 |
| Transaction | `Transaction(ctx, fn)` | 事务糖(回调拿 Service) |

### QueryWrapper[T](api/query-wrapper.md)

| 分类 | 方法 |
|---|---|
| 基础条件 | `Eq` `Ne` `Gt` `Ge` `Lt` `Le` `IsNull` `IsNotNull` |
| 模糊匹配 | `Like` `NotLike` `LikeLeft` `LikeRight` |
| 集合/范围 | `In` `NotIn` `Between` `NotBetween` |
| 连接与嵌套 | `Or` `AndSub` `OrSub` `NotSub` |
| 投影/排序 | `Select` `SelectF` `OrderByAsc` `OrderByDesc` `Order` |
| 分组聚合 | `GroupBy` `Having` |
| JOIN/去重 | `Joins` `Distinct` |
| 关联预载 | `Preload` |
| 行数截断 | `Limit` `Offset` |
| 行锁 | `ForUpdate` `ForShare`(常量 `LockNoWait` `LockSkipLocked`) |
| 类型化条件 | `Where`(配合 [Cond/And/Or/Not](api/typed-field.md)) |
| 其他 | `Clone`(深拷贝) `Apply`(gorm 逃逸口) |

### UpdateWrapper[T](api/update-wrapper.md)

| 分类 | 方法 |
|---|---|
| 赋值 | `Set` `SetExpr` `Incr` `Decr` `SetItems` |
| 条件 | `Eq` `Ne` `Gt` `Ge` `Lt` `Le` `Like` `NotLike` `In` `NotIn` `Between` `NotBetween` `IsNull` `IsNotNull` `Or` `AndSub` `OrSub` `NotSub` `Where` |
| 其他 | `Clone` |

### 类型化字段(docs/api/typed-field.md)

| 分类 | API |
|---|---|
| 字段句柄 | `TypedField[V]` + 17 个具体类型(`StringField` `IntField` `Int64Field` `TimeField` ... `GenericField`) |
| 构造器 | `NewString` `NewInt` ... `NewAny` `NewTyped[V]` |
| 条件方法 | `Eq Ne Gt Ge Lt Le IsNull IsNotNull In NotIn Between NotBetween`(值类型编译期校验) |
| 字符串专属 | `Like NotLike LikeLeft LikeRight`(仅 StringField) |
| 赋值方法 | `SetTo` `Incr` `Decr` |
| 排序 | `Asc` `Desc` |
| 组合子 | `Or(conds...)` `And(conds...)` `Not(conds...)` |

### 原子 upsert(docs/api/upsert.md)

| API | 说明 |
|---|---|
| `InsertOnConflict` | 单条原子 upsert |
| `InsertBatchOnConflict` | 批量原子 upsert |
| `ConflictAction` | 冲突策略(Columns/UpdateFields/UpdateAll/Ignore) |
| `ErrConflictActionInvalid` | 互斥配置错误 |

### 插件(docs/api/plugin.md)

| API | 说明 |
|---|---|
| `RegisterOptimisticLock` | 乐观锁(实体字段 `gorm:"version"` 标签) |
| `RegisterTenant` | 多租户(WHERE 注入 + 插入填充) |
| `WithTenantColumn` / `WithSkipTables` | 租户配置项 |
| `WithTenant(ctx, tenant)` | ctx 携带租户标识 |

### 代码生成(docs/api/codegen.md)

| API | 说明 |
|---|---|
| `gormmaxgen.Config` | 生成配置(Out/Package/Models/Namer/Import) |
| `gormmaxgen.Generate` | 解析模型 → 产出字段集文件 |

### 分页类型(docs/api/mapper.md)

| 类型 | 字段 |
|---|---|
| `Page` | `Current`(从 1 起)、`Size` |
| `PageResult[T]` | `Records` `Total` `Current` `Size` `Pages` |

### 错误变量(docs/api/mapper.md)

| 变量 | 触发场景 |
|---|---|
| `ErrRecordNotFound` | 未找到记录(透传 gorm) |
| `ErrFullTableUpdate` / `ErrFullTableDelete` | 无 WHERE 的更新/删除被拒 |
| `ErrPrimaryKeyMissing` | UpdateByID 主键未设置 |
| `ErrNoSetClause` | Update 缺少 SET 字段 |
| `ErrConflictActionInvalid` | upsert 冲突策略互斥 |
