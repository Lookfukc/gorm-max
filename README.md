# gorm-max

**GORM 最佳搭档,只做增强不做改变,为简化开发、提高效率而生。**

基于 [GORM](https://gorm.io) 的 MyBatis-Plus 风格增强库:不重新造 ORM,在 GORM 之上提供泛型 CRUD、类型安全条件构造、分页、插件体系等增强能力;GORM 的全部能力原样保留,随时可以下沉到原生 API。

```go
// 闭包写法:字段名编译期检查(传错字段编译不过)
users, _ := svc.SelectList(ctx, svc.Query().
	Like(func(u *User) any { return &u.Name }, "tom").
	Gt(func(u *User) any { return &u.Age }, 10).
	OrderByDesc(func(u *User) any { return &u.ID }))

// 类型化字段写法(代码生成):值类型也编译期检查,更短
q := query.User
users, _ = svc.SelectList(ctx, svc.Query().
	Where(q.Name.Like("tom"), q.Age.Gt(10)).
	Order(q.ID.Desc()))
```

## 特性

| 特性 | 说明 |
|---|---|
| **润物无声** | 不改变现有工程——GORM 的模型/方言/Hook/迁移原样保留,引入无侵入 |
| **效率至上** | `NewService[T](db)` 一行即得全套 CRUD + 条件构造 + 分页,一行 SQL 都不用写 |
| **类型安全** | 字段名与值类型双重编译期检查;列名走 schema 白名单 + 方言转义,值全占位符 |
| **丰富功能** | 原子 upsert、乐观锁、多租户、嵌套事务、代码生成器 |

## 从 MyBatis-Plus 迁移:概念对照

| MyBatis-Plus | gorm-max |
|---|---|
| `BaseMapper<T>` / `IService<T>` | `NewMapper[T](db)` / `NewService[T](db)`(组合,无需继承) |
| `QueryWrapper` / `LambdaQueryWrapper` | 闭包式 `Query()` / [类型化字段](docs/guides/typed-field.md) |
| `UpdateWrapper` / `LambdaUpdateWrapper` | `NewUpdate()`(+ `SetItems`) |
| `lambdaQuery()` 链式 | `svc.Query()` 链式 |
| `selectPage` + 分页插件 | `SelectPage(ctx, w, &Page{...})` 内置 |
| `@TableLogic` | 模型加 `gorm.DeletedAt` 字段 |
| `@Version` + 乐观锁插件 | `gorm:"version"` 标签 + [插件](docs/guides/plugin.md) |
| 多租户插件 `TenantLineInnerInterceptor` | [多租户插件](docs/guides/plugin.md)(ctx 注入) |
| 防全表更新与删除插件 | 内置且默认开启(`WithAllowFullTable` 放开) |
| 代码生成器 | [gormmaxgen](docs/guides/codegen.md) |
| `@Transactional` | [`Transaction(ctx, fn)`](docs/guides/transaction.md)(支持嵌套 SavePoint) |

## 安装

```bash
go get gorm-max
# 本地开发(未发布时)用 replace 方式,见 docs/INTEGRATION.md
```

使用示例需要方言驱动(自选):`gorm.io/driver/mysql`、`gorm.io/driver/postgres`、`github.com/glebarez/sqlite` 等。

## 文档

**新用户从 [快速开始](docs/getting-started.md) 入手**——一条龙教程,可直接复制运行。

### 指南

| 章节 | 内容 |
|---|---|
| [CRUD 接口](docs/guides/crud.md) | Mapper/Service 全部方法、更新语义、软删、安全防呆 |
| [条件构造器](docs/guides/wrapper.md) | 闭包式与类型化两套 Wrapper、OR/嵌套分组、投影/JOIN/行锁 |
| [类型化字段](docs/guides/typed-field.md) | 值类型编译期校验的 Lambda 式写法 |
| [分页](docs/guides/page.md) | Page / PageResult |
| [事务](docs/guides/transaction.md) | 事务糖与手动控制 |
| [原子 upsert](docs/guides/upsert.md) | InsertOnConflict、ConflictAction、方言差异 |
| [插件](docs/guides/plugin.md) | 乐观锁、多租户 |
| [代码生成器](docs/guides/codegen.md) | gormmaxgen 配置与类型映射 |

### 参考

| 文档 | 内容 |
|---|---|
| [API 总索引](docs/API.md) | 全部公开 API 速查总表 |
| [Mapper 与 Service](docs/api/mapper.md) | 逐方法:签名/参数/示例 |
| [QueryWrapper](docs/api/query-wrapper.md) · [UpdateWrapper](docs/api/update-wrapper.md) | 全部条件方法 |
| [类型化字段](docs/api/typed-field.md) · [upsert](docs/api/upsert.md) · [插件](docs/api/plugin.md) · [代码生成](docs/api/codegen.md) | 其余 API |
| [接入手册](docs/INTEGRATION.md) | 现有项目(分层架构)接入步骤 |

## 项目结构

```
gorm-max/
├── mapper.go / wrapper.go / column.go / page.go / service.go / field.go / conflict.go
├── plugin/          # 乐观锁、多租户
├── gormmaxgen/      # 代码生成器
├── example/         # model / gen / query(生成产物) / main.go
└── docs/            # 快速开始 / 指南 / API 参考 / 接入手册
```

测试:纯 Go 内存 sqlite(glebarez,免 CGO),覆盖单测 + 集成;`go test ./...` 即可运行。

## 使用注意(速览)

1. wrapper 实例非并发安全:每次请求新建,分叉用 `Clone()`;
2. 嵌入结构体的类型名必须导出(gorm 的 `IsExported` 约定);
3. `UpdateByID` 跳过零值字段(需要写零值用 `Update` + `Set`);
4. 带 `GroupBy`/`Distinct` 的 wrapper 不建议用于 `SelectPage`;
5. 乐观锁只对结构体更新生效,版本 0 视为未启用。

其余注意事项见各指南章节末尾。
