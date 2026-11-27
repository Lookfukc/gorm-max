package gorm_max

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

var (
	ErrRecordNotFound    = gorm.ErrRecordNotFound
	ErrFullTableUpdate   = errors.New("gorm_max: 更新缺少 WHERE 条件(全表更新请用 WithAllowFullTable)")
	ErrFullTableDelete   = errors.New("gorm_max: 删除缺少 WHERE 条件(全表删除请用 WithAllowFullTable)")
	ErrPrimaryKeyMissing = errors.New("gorm_max: 主键未设置")
	ErrNoSetClause       = errors.New("gorm_max: 缺少 SET 字段,请先调用 Set/SetExpr")
)

type MapperOption func(*mapperConfig)

type mapperConfig struct {
	allowFullTable bool
	namer          schema.Namer
}

// WithAllowFullTable 允许无 WHERE 的全表更新/删除,默认禁止。
func WithAllowFullTable() MapperOption {
	return func(c *mapperConfig) { c.allowFullTable = true }
}

// WithNamer 覆盖列名/表名命名策略,默认取 db 配置,再退到 snake_case。
func WithNamer(n schema.Namer) MapperOption {
	return func(c *mapperConfig) { c.namer = n }
}

// Mapper 泛型 CRUD 门面。db 只存"工厂",每次执行派生新 session,避免链式污染。
type Mapper[T any] struct {
	db  *gorm.DB
	res *resolver
	cfg mapperConfig
}

func NewMapper[T any](db *gorm.DB, opts ...MapperOption) *Mapper[T] {
	var cfg mapperConfig
	for _, o := range opts {
		o(&cfg)
	}
	namer := cfg.namer
	if namer == nil {
		namer = db.Config.NamingStrategy // 可能为 nil,resolver 会退到默认
	}
	return &Mapper[T]{db: db, res: resolverOf[T](namer), cfg: cfg}
}

// NewQuery 使用 Mapper 命名策略的查询构造器(与库配置保持一致)。
func (m *Mapper[T]) NewQuery() *QueryWrapper[T] {
	return &QueryWrapper[T]{res: m.res}
}

// NewUpdate 使用 Mapper 命名策略的更新构造器。
func (m *Mapper[T]) NewUpdate() *UpdateWrapper[T] {
	return &UpdateWrapper[T]{QueryWrapper: QueryWrapper[T]{res: m.res}}
}

// Tx 返回绑定到事务的 Mapper(在 db.Transaction 回调里使用)。
func (m *Mapper[T]) Tx(tx *gorm.DB) *Mapper[T] {
	clone := *m
	clone.db = tx
	return &clone
}

// Transaction 事务糖:开启事务并以绑定该事务的 Mapper 执行回调,
// 回调返回 error 自动回滚、nil 自动提交(panic 同样回滚后继续上抛),支持嵌套(内部 SavePoint)。
//
//	err := m.Transaction(ctx, func(tx *Mapper[User]) error {
//		return tx.Insert(ctx, u)
//	})
//
// 注意:回调内所有操作必须走 tx 参数,混用外层 Mapper 的语句会脱离事务。
func (m *Mapper[T]) Transaction(ctx context.Context, fn func(tx *Mapper[T]) error) error {
	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(m.Tx(tx))
	})
}

func (m *Mapper[T]) session(ctx context.Context) *gorm.DB {
	return m.db.Session(&gorm.Session{NewDB: true}).WithContext(ctx).Model(new(T))
}

// whereSQL 取 wrapper 的 WHERE 片段,执行时注入方言转义。
func (m *Mapper[T]) whereSQL(w *QueryWrapper[T]) (string, []any) {
	if w == nil {
		return "", nil
	}
	w.dialect = m.db.Dialector
	return w.buildSQL()
}

// apply 把 wrapper 翻译到 gorm 链上。
func (m *Mapper[T]) apply(s *gorm.DB, w *QueryWrapper[T]) *gorm.DB {
	if w == nil {
		return s
	}
	if sql, args := m.whereSQL(w); sql != "" {
		s = s.Where(sql, args...)
	}
	if g := w.buildGroupSQL(); g != "" {
		s = s.Group(g)
	}
	if h, hArgs := w.buildHavingSQL(); h != "" {
		s = s.Having(h, hArgs...)
	}
	for _, j := range w.joins {
		s = s.Joins(j.sql, j.args...)
	}
	if sel := w.buildSelectWithDistinct(); sel != "" {
		s = s.Select(sel)
	}
	for _, p := range w.preloads {
		s = s.Preload(p.assoc, p.args...)
	}
	for _, ord := range w.buildOrderSQL() {
		s = s.Order(ord)
	}
	if w.limit != nil {
		s = s.Limit(int(*w.limit))
	}
	if w.offset != nil {
		s = s.Offset(int(*w.offset))
	}
	for _, lk := range w.locking {
		s = s.Clauses(lk)
	}
	for _, fn := range w.applies {
		s = fn(s)
	}
	return s
}

// ---- 写 ----

func (m *Mapper[T]) Insert(ctx context.Context, entity *T) error {
	return m.session(ctx).Create(entity).Error
}

func (m *Mapper[T]) InsertBatch(ctx context.Context, entities []*T, batchSize ...int) error {
	if len(entities) == 0 {
		return nil
	}
	size := 500
	if len(batchSize) > 0 && batchSize[0] > 0 {
		size = batchSize[0]
	}
	return m.session(ctx).CreateInBatches(entities, size).Error
}

func (m *Mapper[T]) UpdateByID(ctx context.Context, entity *T) (int64, error) {
	if _, ok := m.res.pkValue(entity); !ok {
		return 0, ErrPrimaryKeyMissing
	}
	// entity 直接作为 dest:gorm 会用它自带的主键做 WHERE,并跳过零值字段
	s := m.db.Session(&gorm.Session{NewDB: true}).WithContext(ctx).Updates(entity)
	return s.RowsAffected, s.Error
}

func (m *Mapper[T]) Update(ctx context.Context, uw *UpdateWrapper[T]) (int64, error) {
	uw.dialect = m.db.Dialector // SET 表达式的列名转义依赖方言,须先于 setsMap
	sets := uw.setsMap()
	if len(sets) == 0 {
		return 0, ErrNoSetClause
	}
	sql, args := m.whereSQL(&uw.QueryWrapper)
	var s *gorm.DB
	if sql == "" {
		if !m.cfg.allowFullTable {
			return 0, ErrFullTableUpdate
		}
		s = m.session(ctx)
	} else {
		s = m.session(ctx).Where(sql, args...)
	}
	s = s.Model(new(T)).Updates(sets)
	return s.RowsAffected, s.Error
}

func (m *Mapper[T]) DeleteByID(ctx context.Context, id any) (int64, error) {
	s := m.session(ctx).Delete(new(T), id) // 内联条件走主键
	return s.RowsAffected, s.Error
}

func (m *Mapper[T]) Delete(ctx context.Context, w *QueryWrapper[T]) (int64, error) {
	sql, args := m.whereSQL(w)
	var s *gorm.DB
	if sql == "" {
		if !m.cfg.allowFullTable {
			return 0, ErrFullTableDelete
		}
		s = m.session(ctx)
	} else {
		s = m.session(ctx).Where(sql, args...)
	}
	s = s.Delete(new(T))
	return s.RowsAffected, s.Error
}

// ---- 读 ----

func (m *Mapper[T]) SelectByID(ctx context.Context, id any) (*T, error) {
	var out T
	if err := m.session(ctx).First(&out, id).Error; err != nil {
		return nil, err // 未找到时为 gorm.ErrRecordNotFound
	}
	return &out, nil
}

func (m *Mapper[T]) SelectOne(ctx context.Context, w *QueryWrapper[T]) (*T, error) {
	var out T
	if err := m.apply(m.session(ctx), w).Limit(1).Take(&out).Error; err != nil {
		return nil, err
	}
	return &out, nil
}

func (m *Mapper[T]) SelectList(ctx context.Context, w *QueryWrapper[T]) ([]*T, error) {
	var out []*T
	if err := m.apply(m.session(ctx), w).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (m *Mapper[T]) SelectCount(ctx context.Context, w *QueryWrapper[T]) (int64, error) {
	var n int64
	err := m.apply(m.session(ctx), w).Count(&n).Error
	return n, err
}

// Exists 是否存在匹配记录。
func (m *Mapper[T]) Exists(ctx context.Context, w *QueryWrapper[T]) (bool, error) {
	n, err := m.SelectCount(ctx, w)
	return n > 0, err
}

// DB 返回模型作用域的全新 gorm 会话,用于 wrapper 覆盖不到的复杂查询
// (join/窗口函数/CTE/聚合扫描等)。每次调用都是独立会话,返回值请勿保存复用。
func (m *Mapper[T]) DB(ctx context.Context) *gorm.DB {
	return m.db.Session(&gorm.Session{NewDB: true}).WithContext(ctx).Model(new(T))
}

func (m *Mapper[T]) SelectPage(ctx context.Context, w *QueryWrapper[T], p *Page) (*PageResult[T], error) {
	if p == nil {
		p = &Page{Current: 1, Size: 10}
	}
	total, err := m.SelectCount(ctx, w)
	if err != nil {
		return nil, err
	}
	res := &PageResult[T]{Total: total, Current: p.Current, Size: p.Size}
	if p.Size > 0 {
		res.Pages = (total + p.Size - 1) / p.Size
	}
	if total > 0 {
		var records []*T
		s := m.apply(m.session(ctx), w).Limit(int(p.Size)).Offset(int(p.offset()))
		if err := s.Find(&records).Error; err != nil {
			return nil, err
		}
		res.Records = records
	}
	return res, nil
}
