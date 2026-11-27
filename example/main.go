//go:generate go run ./gen/main.go

package main

import (
	"context"
	"fmt"
	"log"

	"github.com/Lookfukc/gorm-max"
	"github.com/Lookfukc/gorm-max/example/model"
	"github.com/Lookfukc/gorm-max/example/query"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func main() {
	db, err := gorm.Open(mysql.Open(
		"root:pass@tcp(127.0.0.1:3306)/demo?charset=utf8mb4&parseTime=true&loc=Local"), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}); err != nil {
		log.Fatal(err)
	}

	svc := gorm_max.NewService[model.User](db)
	ctx := context.Background()

	// 1. 插入 / 批量插入
	_ = svc.Insert(ctx, &model.User{Name: "tom", Age: 20, Email: "tom@x.com"})
	_ = svc.InsertBatch(ctx, []*model.User{
		{Name: "jerry", Age: 18}, {Name: "spike", Age: 30}, {Name: "tyke", Age: 5},
	})

	// 2. 按主键查询
	u, err := svc.SelectByID(ctx, 1)
	fmt.Println("byId:", u, err)

	// 3. 条件查询:name LIKE '%tom%' AND age > 10,按 id 倒序
	users, _ := svc.SelectList(ctx, svc.Query().
		Like(func(x *model.User) any { return &x.Name }, "tom").
		Gt(func(x *model.User) any { return &x.Age }, 10).
		OrderByDesc(func(x *model.User) any { return &x.ID }))
	fmt.Println("list:", len(users))

	// 4. 嵌套条件:name LIKE '%j%' AND (age < 10 OR age > 25)
	nested := svc.Query().
		Like(func(x *model.User) any { return &x.Name }, "j").
		AndSub(func(q *gorm_max.QueryWrapper[model.User]) *gorm_max.QueryWrapper[model.User] {
			return q.Lt(func(x *model.User) any { return &x.Age }, 10).
				Or().
				Gt(func(x *model.User) any { return &x.Age }, 25)
		})
	users, _ = svc.SelectList(ctx, nested)
	fmt.Println("nested:", len(users))

	// 5. 分页
	page, _ := svc.SelectPage(ctx,
		svc.Query().Gt(func(x *model.User) any { return &x.Age }, 0),
		&gorm_max.Page{Current: 1, Size: 2})
	fmt.Printf("page: total=%d pages=%d size=%d\n", page.Total, page.Pages, len(page.Records))

	// 6. 更新:SET age = age + 1 WHERE name = 'tom'
	n, _ := svc.Update(ctx, svc.NewUpdate().
		SetExpr(func(x *model.User) any { return &x.Age }, "age + ?", 1).
		Eq(func(x *model.User) any { return &x.Name }, "tom"))
	fmt.Println("updated:", n)

	// 7. SaveOrUpdate(有主键 → 更新)
	if u != nil {
		u.Age = 99
		_ = svc.SaveOrUpdate(ctx, u)
	}

	// 8. 删除(有 DeletedAt 字段时自动软删)
	n, _ = svc.Delete(ctx, svc.Query().Eq(func(x *model.User) any { return &x.Name }, "tyke"))
	fmt.Println("deleted:", n)

	// 9. 事务
	_ = db.Transaction(func(tx *gorm.DB) error {
		return svc.Tx(tx).Insert(ctx, &model.User{Name: "in-tx", Age: 1})
	})

	// 10. 编译期类型校验写法(可选):值类型不匹配直接编译报错
	_, _ = svc.SelectList(ctx, gorm_max.Eq(svc.Query(), func(x *model.User) string { return x.Name }, "tom"))

	// 11. 插件(可选,import "github.com/Lookfukc/gorm-max/plugin"),打开数据库后注册一次:
	//   乐观锁:实体字段加 gorm:"version" 标签 → 更新自动 SET version=旧+1 / WHERE version=旧,0 行即冲突
	//     _ = plugin.RegisterOptimisticLock(db)
	//   多租户:模型带 tenant_id 列,ctx 携带租户后自动注入/填充
	//     _ = plugin.RegisterTenant(db)
	//     ctx = plugin.WithTenant(ctx, "t001")

	// 12. M5 类型化字段(生成物见 query/fields_gen.go,go generate ./... 重新生成):
	//     值类型编译期校验 + 无运行时解析,与闭包 API 可混用
	q := query.User
	users2, _ := svc.SelectList(ctx, svc.Query().
		Where(q.Name.Like("tom"), q.Age.Gt(10)).
		Order(q.ID.Desc()))
	fmt.Println("typed fields:", len(users2))
}
