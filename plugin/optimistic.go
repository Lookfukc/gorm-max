// Package plugin 提供 MyBatis-Plus 风格的 GORM 插件(乐观锁、多租户)。
//
// 打开数据库后注册一次,作用于该 *gorm.DB 的所有会话:
//
//	db, _ := gorm.Open(...)
//	plugin.RegisterOptimisticLock(db)          // 乐观锁
//	plugin.RegisterTenant(db)                  // 多租户
package plugin

import (
	"fmt"
	"reflect"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

// RegisterOptimisticLock 注册乐观锁插件(对应 MP 的 @Version)。
//
// 约定:实体版本字段加 gorm:"version" 标签,如:
//
//	Version int `gorm:"version"`
//
// 行为:以结构体为目标的更新(UpdateByID/SaveOrUpdate)自动改写为
//
//	SET version = 旧值 + 1 ... WHERE id = ? AND version = 旧值
//
// 受影响行数为 0 即版本冲突,由调用方根据 RowsAffected 判断。
// 版本值为 0 视为未启用(等价 MP 的 version == null);建议插入时显式置 1 或用 DB 默认值。
// wrapper 的 map 更新(Update 方法)不参与乐观锁。
func RegisterOptimisticLock(db *gorm.DB) error {
	return db.Callback().Update().Before("gorm:update").
		Register("gorm_max:optimistic_lock", optimisticLockCallback)
}

func optimisticLockCallback(db *gorm.DB) {
	stmt := db.Statement
	if stmt == nil || stmt.Schema == nil || stmt.Dest == nil {
		return
	}
	rv := reflect.Indirect(reflect.ValueOf(stmt.Dest))
	if rv.Kind() != reflect.Struct || !rv.CanAddr() {
		return // 仅结构体目标;map/slice 更新不参与
	}
	vf := findTaggedField(stmt.Schema, "VERSION")
	if vf == nil {
		return
	}
	old, zero := vf.ValueOf(stmt.Context, rv)
	if zero {
		return // 版本 0 视为未启用
	}
	oldV, err := toInt64(old)
	if err != nil {
		db.AddError(err)
		return
	}
	// 1) 目标版本 +1,gorm 构建结构体更新时会把它并入 SET
	if err := vf.Set(stmt.Context, rv, oldV+1); err != nil {
		db.AddError(err)
		return
	}
	// 2) 追加 WHERE version = 旧值
	stmt.AddClause(clause.Where{Exprs: []clause.Expression{
		clause.Eq{Column: clause.Column{Name: vf.DBName}, Value: oldV},
	}})
}

// findTaggedField 按 gorm 标签找字段(TagSettings 的 key 已被 gorm 转为大写)。
func findTaggedField(s *schema.Schema, tagKey string) *schema.Field {
	for _, f := range s.Fields {
		if v, ok := f.TagSettings[tagKey]; ok && v != "" && v != "FALSE" {
			return f
		}
	}
	return nil
}

func toInt64(v any) (int64, error) {
	rv := reflect.Indirect(reflect.ValueOf(v))
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(rv.Uint()), nil
	}
	return 0, fmt.Errorf("gorm_max/plugin: 版本字段类型不支持: %T", v)
}
