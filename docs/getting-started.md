# 快速开始

通过一个简单的 Demo 快速上手 gorm-max,你将学到:

- 配置依赖
- 创建实体类
- 创建 Service
- 使用 gorm-max 强大的 CRUD、条件构造与分页能力

> 完整可运行的示例代码见仓库 [`example/`](../example/) 目录。

## 1. 概述

**前置要求:**

- Go 1.25+
- 了解 [GORM](https://gorm.io) 基础(连接数据库、模型定义)

**本文示例的数据形态**(教程用内存 SQLite 全程自动建表,无需手工执行;此处仅供理解):

```sql
CREATE TABLE users (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       VARCHAR(64),
    age        INTEGER,
    email      VARCHAR(128),
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME
);
```

## 2. 初始化工程

```bash
mkdir demo && cd demo
go mod init demo
```

## 3. 添加依赖

```bash
go get github.com/Lookfukc/gorm-max
go get github.com/glebarez/sqlite   # 教程用内存 SQLite(免 CGO);生产替换为 gorm.io/driver/mysql 等
```

## 4. 配置

打开数据库并让 gorm-max 接管它。**不需要写任何 SQL、不需要配置文件**,打开即用:

```go
package main

import (
    "github.com/glebarez/sqlite"
    "gorm.io/gorm"
    "gorm.io/gorm/logger"
)

func mustOpen() *gorm.DB {
    db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
        Logger: logger.Default.LogMode(logger.Info), // 控制台打印生成的 SQL,学习期建议开启
    })
    if err != nil {
        panic(err)
    }
    return db
}
```

> 生产环境把 `sqlite.Open(...)` 换成 `mysql.Open(dsn)` / `postgres.Open(dsn)`,其余代码**一行不改**。

## 5. 编码

### 实体类

```go
type User struct {
    ID        int64          `gorm:"primaryKey"`
    Name      string         `gorm:"size:64;index"`
    Age       int
    Email     string         `gorm:"size:128"`
    CreatedAt time.Time
    UpdatedAt time.Time
    DeletedAt gorm.DeletedAt `gorm:"index"` // 逻辑删除,等价 MP 的 @TableLogic
}
```

### 创建 Service

MP 中你继承 `BaseMapper<T>` / `ServiceImpl`;gorm-max 中你**组合**一个绑定到模型类型的泛型 `Service[T]`:

```go
svc := gorm_max.NewService[User](db) // 增删改查、分页、事务全部就绪
```

## 6. 开始使用

下面是完整的 `main.go`,可直接复制运行:

```go
package main

import (
	"context"
	"fmt"
	"time"

	gorm_max "github.com/Lookfukc/gorm-max"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type User struct {
	ID        int64          `gorm:"primaryKey"`
	Name      string         `gorm:"size:64;index"`
	Age       int
	Email     string         `gorm:"size:128"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func main() {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		panic(err)
	}
	// 自动建表(对应 MP 教程中手工执行 schema.sql)
	if err := db.AutoMigrate(&User{}); err != nil {
		panic(err)
	}

	svc := gorm_max.NewService[User](db)
	ctx := context.Background()

	// ---- 插入 ----
	_ = svc.Insert(ctx, &User{Name: "tom", Age: 20, Email: "tom@x.com"})
	_ = svc.InsertBatch(ctx, []*User{
		{Name: "jerry", Age: 18},
		{Name: "spike", Age: 30},
		{Name: "tyke", Age: 5},
	})

	// ---- 查询:按主键 ----
	u, err := svc.SelectByID(ctx, 1)
	fmt.Println(u, err) // &{1 tom 20 tom@x.com ...} <nil>

	// ---- 查询:条件构造(name LIKE '%tom%' AND age > 10,按 id 倒序)----
	users, _ := svc.SelectList(ctx, svc.Query().
		Like(func(x *User) any { return &x.Name }, "tom").
		Gt(func(x *User) any { return &x.Age }, 10).
		OrderByDesc(func(x *User) any { return &x.ID }))
	fmt.Println(len(users)) // 1

	// ---- 分页 ----
	page, _ := svc.SelectPage(ctx,
		svc.Query().Gt(func(x *User) any { return &x.Age }, 0),
		&gorm_max.Page{Current: 1, Size: 2})
	fmt.Printf("total=%d pages=%d records=%d\n", page.Total, page.Pages, len(page.Records)) // total=4 pages=2 records=2

	// ---- 更新:SET age = age + 1 WHERE name = 'tom' ----
	n, _ := svc.Update(ctx, svc.NewUpdate().
		SetExpr(func(x *User) any { return &x.Age }, "age + ?", 1).
		Eq(func(x *User) any { return &x.Name }, "tom"))
	fmt.Println(n) // 1

	// ---- 删除(有 DeletedAt 字段 → 自动软删)----
	n, _ = svc.Delete(ctx, svc.Query().Eq(func(x *User) any { return &x.Name }, "tyke"))
	fmt.Println(n) // 1
}
```

控制台(开启 `logger.Info` 后)可以看到每条语句生成的 SQL,例如条件查询:

```sql
SELECT * FROM `users` WHERE (`name` LIKE "%tom%" AND `age` > 10) AND `users`.`deleted_at` IS NULL ORDER BY `id` DESC
```

> 与 MP 一致:查询条件为空(直接传 `svc.Query()`)即查全表。

## 7. 小结

引依赖 → 打开数据库 → `NewService[T](db)` → 即用。**甚至一行 SQL 都不用写**,而 GORM 的全部能力(关联、Hook、迁移、多方言)原样保留,随时可下沉到原生 API。

下一步:

| 想了解 | 阅读 |
|---|---|
| 全部 CRUD 方法与事务 | [CRUD 接口](guides/crud.md) |
| 条件构造器(闭包式 / 类型化字段) | [条件构造器](guides/wrapper.md) |
| 值类型编译期校验的 Lambda 写法 | [类型化字段](guides/typed-field.md) |
| 乐观锁、多租户插件 | [插件](guides/plugin.md) |
| 现有项目如何接入 | [接入手册](INTEGRATION.md) |
