package gorm_max

import (
	"context"
	"errors"

	"gorm.io/gorm/clause"
)

// ConflictAction 插入冲突(唯一键冲突)时的处理策略,用于数据库级原子 upsert。
// 零值等价于 Ignore(冲突即忽略,不覆盖已有数据),是最安全的默认。
type ConflictAction struct {
	// Columns 冲突判定列;仅 PostgreSQL/SQLite 支持,MySQL 忽略此设置(按任意唯一键判定)。
	Columns []string
	// UpdateFields 冲突时更新的列(字段选择器),例如:
	//
	//	ConflictAction{UpdateFields: []any{func(x *User) any { return &x.Age }}}
	UpdateFields []any
	// UpdateAll 冲突时更新除主键、自动创建时间外的全部列(UpdatedAt 会刷新)。
	// 未指定 Columns 时,gorm 默认以主键作为冲突判定列;
	// 若唯一键不是主键(如业务唯一码),必须显式指定 Columns。
	UpdateAll bool
	// Ignore 冲突时忽略本次插入;与 UpdateAll/UpdateFields 互斥。
	Ignore bool
}

// ErrConflictActionInvalid ConflictAction 配置互斥(Ignore/UpdateAll/UpdateFields 至多选一)。
var ErrConflictActionInvalid = errors.New("gorm_max: ConflictAction 配置冲突(Ignore/UpdateAll/UpdateFields 至多选一)")

// buildOnConflict 转换为 gorm 冲突子句。
// 字段选择器在此解析为列名,渲染交给 gorm(标识符经方言转义,值经占位符,无注入面)。
func (m *Mapper[T]) buildOnConflict(act ConflictAction) (clause.OnConflict, error) {
	if (act.Ignore && (act.UpdateAll || len(act.UpdateFields) > 0)) ||
		(act.UpdateAll && len(act.UpdateFields) > 0) {
		return clause.OnConflict{}, ErrConflictActionInvalid
	}
	oc := clause.OnConflict{}
	for _, col := range act.Columns {
		oc.Columns = append(oc.Columns, clause.Column{Name: col})
	}
	switch {
	case act.UpdateAll:
		oc.UpdateAll = true
	case len(act.UpdateFields) > 0:
		cols := make([]string, 0, len(act.UpdateFields))
		for _, f := range act.UpdateFields {
			cols = append(cols, m.res.resolve(f).Name)
		}
		oc.DoUpdates = clause.AssignmentColumns(cols)
	default:
		oc.DoNothing = true // 零值或显式 Ignore → 冲突忽略
	}
	return oc, nil
}

// InsertOnConflict 数据库级原子 upsert:唯一键冲突时按策略更新或忽略,无应用层竞态。
//
// MySQL 生成 ON DUPLICATE KEY UPDATE(忽略策略自动降级为主键自赋值的 no-op);
// PostgreSQL/SQLite 生成 ON CONFLICT [...] DO UPDATE SET / DO NOTHING。
//
// 返回受影响行数:新插入 1;冲突更新 1;冲突忽略 0(MySQL 默认统计口径下同样适用)。
// 注意:冲突走更新分支时,自增主键不会回填到实体(该行本已存在)。
func (m *Mapper[T]) InsertOnConflict(ctx context.Context, entity *T, act ConflictAction) (int64, error) {
	oc, err := m.buildOnConflict(act)
	if err != nil {
		return 0, err
	}
	s := m.session(ctx).Clauses(oc).Create(entity)
	return s.RowsAffected, s.Error
}

// InsertBatchOnConflict 批量 upsert,语义同 InsertOnConflict,按 batchSize 分批执行。
func (m *Mapper[T]) InsertBatchOnConflict(ctx context.Context, entities []*T, act ConflictAction, batchSize ...int) (int64, error) {
	if len(entities) == 0 {
		return 0, nil
	}
	oc, err := m.buildOnConflict(act)
	if err != nil {
		return 0, err
	}
	size := 500
	if len(batchSize) > 0 && batchSize[0] > 0 {
		size = batchSize[0]
	}
	s := m.session(ctx).Clauses(oc).CreateInBatches(entities, size)
	return s.RowsAffected, s.Error
}
