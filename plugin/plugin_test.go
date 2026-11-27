package plugin_test

import (
	"context"
	"fmt"
	"testing"

	"gorm-max"
	"gorm-max/plugin"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type versionUser struct {
	ID      int64  `gorm:"primaryKey"`
	Name    string `gorm:"size:64"`
	Version int    `gorm:"version"`
}

type tenantOrder struct {
	ID       int64  `gorm:"primaryKey"`
	TenantID string `gorm:"size:32;index"`
	Amount   int
}

var pluginDBSeq int

func newPluginDB(t *testing.T, models ...any) *gorm.DB {
	t.Helper()
	pluginDBSeq++
	dsn := fmt.Sprintf("file:plugindb%d?mode=memory&cache=shared", pluginDBSeq)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestOptimisticLock(t *testing.T) {
	db := newPluginDB(t, &versionUser{}, &tenantOrder{})
	if err := plugin.RegisterOptimisticLock(db); err != nil {
		t.Fatalf("register: %v", err)
	}
	m := gorm_max.NewMapper[versionUser](db)
	ctx := context.Background()

	u := &versionUser{Name: "a", Version: 1}
	if err := m.Insert(ctx, u); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// 正常更新:version 1 → 2
	u.Name = "a2"
	n, err := m.UpdateByID(ctx, u)
	if err != nil || n != 1 {
		t.Fatalf("update: %v %d", err, n)
	}
	if u.Version != 2 {
		t.Fatalf("实体版本应回填为 2: %d", u.Version)
	}
	if got, _ := m.SelectByID(ctx, u.ID); got.Version != 2 || got.Name != "a2" {
		t.Fatalf("db 版本应为 2: %+v", got)
	}

	// 旧版本冲突:DB 已是 2,仍用 Version=1 提交
	stale := &versionUser{ID: u.ID, Name: "a3", Version: 1}
	if n, err := m.UpdateByID(ctx, stale); err != nil || n != 0 {
		t.Fatalf("旧版本应更新 0 行: %v %d", err, n)
	}

	// 版本 0 视为未启用:不加锁,正常更新
	plain := &versionUser{ID: u.ID, Name: "a4"}
	if n, err := m.UpdateByID(ctx, plain); err != nil || n != 1 {
		t.Fatalf("version=0 应跳过乐观锁: %v %d", err, n)
	}

	// 无 version 字段的模型不受影响
	mo := gorm_max.NewMapper[tenantOrder](db)
	o := &tenantOrder{TenantID: "t9", Amount: 1}
	_ = mo.Insert(ctx, o)
	if n, err := mo.UpdateByID(ctx, o); err != nil || n != 1 {
		t.Fatalf("无 version 字段模型: %v %d", err, n)
	}
}

func TestTenant(t *testing.T) {
	db := newPluginDB(t, &tenantOrder{})
	if err := plugin.RegisterTenant(db); err != nil {
		t.Fatalf("register: %v", err)
	}
	m := gorm_max.NewMapper[tenantOrder](db)
	ctx1 := plugin.WithTenant(context.Background(), "t1")
	ctx2 := plugin.WithTenant(context.Background(), "t2")

	// 插入自动填充租户列
	if err := m.InsertBatch(ctx1, []*tenantOrder{{Amount: 1}, {Amount: 2}}); err != nil {
		t.Fatalf("insertBatch: %v", err)
	}
	if err := m.Insert(ctx2, &tenantOrder{Amount: 3}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	// 显式赋值优先于自动填充
	if err := m.Insert(ctx1, &tenantOrder{TenantID: "t9", Amount: 9}); err != nil {
		t.Fatalf("insert explicit: %v", err)
	}

	// 查询按租户隔离(t1 自身 2 行;显式 t9 的行不属于 t1,不可见)
	rows1, err := m.SelectList(ctx1, nil)
	if err != nil || len(rows1) != 2 {
		t.Fatalf("t1 应查到 2 行: %v %d", err, len(rows1))
	}
	for _, r := range rows1 {
		if r.TenantID == "t2" {
			t.Fatalf("t1 查到了 t2 的数据: %+v", r)
		}
	}
	if rows2, _ := m.SelectList(ctx2, nil); len(rows2) != 1 || rows2[0].Amount != 3 {
		t.Fatalf("t2 应查到 1 行: %+v", rows2)
	}
	// 无租户 ctx 不注入:能查全部
	if all, _ := m.SelectList(context.Background(), nil); len(all) != 4 {
		t.Fatalf("无租户应查到全部 4 行: %d", len(all))
	}

	// 更新限定租户
	n, err := m.Update(ctx2, m.NewUpdate().
		Set(func(x *tenantOrder) any { return &x.Amount }, 30).
		Eq(func(x *tenantOrder) any { return &x.Amount }, 3))
	if err != nil || n != 1 {
		t.Fatalf("t2 update: %v %d", err, n)
	}
	if got, _ := m.SelectByID(ctx1, 1); got.Amount != 1 {
		t.Fatalf("t1 数据不应被 t2 更新影响: %+v", got)
	}

	// 删除限定租户
	n, err = m.Delete(ctx1, m.NewQuery().Eq(func(x *tenantOrder) any { return &x.Amount }, 1))
	if err != nil || n != 1 {
		t.Fatalf("t1 delete: %v %d", err, n)
	}
	if left, _ := m.SelectCount(ctx1, nil); left != 1 {
		t.Fatalf("t1 删除后应剩 1 行: %d", left)
	}
}

func TestTenantSkipTables(t *testing.T) {
	db := newPluginDB(t, &tenantOrder{})
	if err := plugin.RegisterTenant(db, plugin.WithSkipTables("tenant_orders")); err != nil {
		t.Fatalf("register: %v", err)
	}
	m := gorm_max.NewMapper[tenantOrder](db)
	ctx := plugin.WithTenant(context.Background(), "t1")

	// 整表被跳过:不填充、不注入
	if err := m.Insert(ctx, &tenantOrder{Amount: 1}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	rows, err := m.SelectList(ctx, nil)
	if err != nil || len(rows) != 1 || rows[0].TenantID != "" {
		t.Fatalf("跳过表应不注入租户: %v %+v", err, rows)
	}
}

func TestTenantCustomColumn(t *testing.T) {
	type customOrg struct {
		ID   int64  `gorm:"primaryKey"`
		Org  string `gorm:"size:32;column:org_id"`
		Name string
	}
	db := newPluginDB(t, &customOrg{})
	if err := plugin.RegisterTenant(db, plugin.WithTenantColumn("org_id")); err != nil {
		t.Fatalf("register: %v", err)
	}
	m := gorm_max.NewMapper[customOrg](db)
	ctx := plugin.WithTenant(context.Background(), "org-a")

	if err := m.Insert(ctx, &customOrg{Name: "n1"}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	rows, err := m.SelectList(ctx, nil)
	if err != nil || len(rows) != 1 || rows[0].Org != "org-a" {
		t.Fatalf("自定义列名: %v %+v", err, rows)
	}
}
