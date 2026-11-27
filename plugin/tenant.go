package plugin

import (
	"context"
	"reflect"
	"slices"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

type tenantCtxKey struct{}

// WithTenant 把租户标识放入 ctx,配合 RegisterTenant 使用。
func WithTenant(ctx context.Context, tenant string) context.Context {
	return context.WithValue(ctx, tenantCtxKey{}, tenant)
}

func tenantFrom(ctx context.Context) (string, bool) {
	v, _ := ctx.Value(tenantCtxKey{}).(string)
	return v, v != ""
}

// TenantConfig 多租户配置。
type TenantConfig struct {
	Column     string   // 租户列名,默认 tenant_id
	SkipTables []string // 忽略的表(如租户表自身、全局字典表)
}

// TenantOption 多租户选项。
type TenantOption func(*TenantConfig)

// WithTenantColumn 覆盖租户列名。
func WithTenantColumn(col string) TenantOption {
	return func(c *TenantConfig) { c.Column = col }
}

// WithSkipTables 追加忽略的表。
func WithSkipTables(tables ...string) TenantOption {
	return func(c *TenantConfig) { c.SkipTables = append(c.SkipTables, tables...) }
}

// RegisterTenant 注册多租户插件(对应 MP 的 TenantLineInnerInterceptor)。
//
// 约定:模型具有租户列(默认列名 tenant_id)。
//   - ctx 携带租户时:查询/更新/删除自动追加 WHERE tenant_id = ?;
//     插入时若实体该列为零值则自动填充(显式赋值优先)。
//   - ctx 无租户时不注入,便于运维/后台任务显式操作。
//   - 模型没有租户列的表自动跳过;SkipTables 显式跳过。
//
// 注意:若业务 wrapper 里已手写租户条件,会出现重复条件(不影响正确性,只影响可读性)。
func RegisterTenant(db *gorm.DB, opts ...TenantOption) error {
	cfg := TenantConfig{Column: "tenant_id"}
	for _, o := range opts {
		o(&cfg)
	}
	cb := db.Callback()
	if err := cb.Create().Before("gorm:create").
		Register("gorm_max:tenant_create", tenantCreateCallback(cfg)); err != nil {
		return err
	}
	if err := cb.Query().Before("gorm:query").
		Register("gorm_max:tenant_query", tenantWhereCallback(cfg)); err != nil {
		return err
	}
	if err := cb.Update().Before("gorm:update").
		Register("gorm_max:tenant_update", tenantWhereCallback(cfg)); err != nil {
		return err
	}
	return cb.Delete().Before("gorm:delete").
		Register("gorm_max:tenant_delete", tenantWhereCallback(cfg))
}

func tenantCreateCallback(cfg TenantConfig) func(*gorm.DB) {
	return func(db *gorm.DB) {
		tenant, ok := tenantFrom(db.Statement.Context)
		if !ok || skipTable(db, cfg) {
			return
		}
		f := db.Statement.Schema.FieldsByDBName[cfg.Column]
		if f == nil { // 模型没有租户列
			return
		}
		rv := reflect.Indirect(reflect.ValueOf(db.Statement.Dest))
		switch rv.Kind() {
		case reflect.Struct:
			fillTenant(db, f, rv, tenant)
		case reflect.Slice, reflect.Array: // 批量插入
			for i := 0; i < rv.Len(); i++ {
				fillTenant(db, f, rv.Index(i), tenant)
			}
		}
	}
}

func tenantWhereCallback(cfg TenantConfig) func(*gorm.DB) {
	return func(db *gorm.DB) {
		tenant, ok := tenantFrom(db.Statement.Context)
		if !ok || skipTable(db, cfg) {
			return
		}
		if _, has := db.Statement.Schema.FieldsByDBName[cfg.Column]; !has {
			return // 模型没有租户列
		}
		db.Statement.AddClause(clause.Where{Exprs: []clause.Expression{
			clause.Eq{Column: clause.Column{Name: cfg.Column}, Value: tenant},
		}})
	}
}

// fillTenant 仅在零值时填充,尊重显式赋值。
func fillTenant(db *gorm.DB, f *schema.Field, v reflect.Value, tenant string) {
	v = reflect.Indirect(v)
	if v.Kind() != reflect.Struct || !v.CanAddr() {
		return
	}
	if _, zero := f.ValueOf(db.Statement.Context, v); !zero {
		return
	}
	if err := f.Set(db.Statement.Context, v, tenant); err != nil {
		db.AddError(err)
	}
}

func skipTable(db *gorm.DB, cfg TenantConfig) bool {
	return db.Statement == nil || db.Statement.Schema == nil ||
		slices.Contains(cfg.SkipTables, db.Statement.Schema.Table)
}
