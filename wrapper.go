package gorm_max

import (
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Field 字段选择器:闭包形式读写模型字段。
// 推荐 return &u.Name(指针写法,识别 O(1));return u.Name 亦可。
type Field[T any] func(*T) any

type logic uint8

const (
	logicAnd logic = iota
	logicOr
)

type cond[T any] struct {
	logic  logic            // 与前一条的连接方式
	sub    *QueryWrapper[T] // 嵌套分组,非空时忽略 op/col/vals
	negate bool             // 分组取反(NOT (...))
	op     string           // 含占位符的 SQL 片段,如 " = ?" / " IN (?,?)"
	col    *Column
	vals   []any
}

func (c cond[T]) clone() cond[T] {
	c2 := c
	c2.vals = cloneSlice(c.vals)
	if c.sub != nil {
		c2.sub = c.sub.Clone()
	}
	return c2
}

func cloneSlice[S ~[]E, E any](s S) S {
	if s == nil {
		return nil
	}
	out := make(S, len(s))
	copy(out, s)
	return out
}

// QueryWrapper 查询条件构造器。
// 注意:非并发安全,不要跨 goroutine 复用同一个实例。
type QueryWrapper[T any] struct {
	res          *resolver
	dialect      gorm.Dialector // 由 Mapper 执行时注入,用于列名转义
	conds        []cond[T]
	selects      []*Column
	orders       []order
	groups       []*Column
	havings      []havingClause
	preloads     []preloadClause
	joins        []joinClause
	distinct     bool
	distinctCols []*Column
	locking      []clause.Locking
	limit        *int64
	offset       *int64
	applies      []func(*gorm.DB) *gorm.DB
	pendingOr    bool
}

type order struct {
	col *Column
	dir string
}

type havingClause struct {
	sql  string
	args []any
}

type preloadClause struct {
	assoc string
	args  []any
}

type joinClause struct {
	sql  string
	args []any
}

// NewQuery 独立创建(默认 snake_case 命名);在 Mapper/Service 上请用 NewQuery 方法。
func NewQuery[T any]() *QueryWrapper[T] {
	return &QueryWrapper[T]{res: resolverOf[T](nil)}
}

// ---- 基础条件 ----

func (w *QueryWrapper[T]) Eq(f Field[T], v any) *QueryWrapper[T] { return w.add(" = ?", f, v) }
func (w *QueryWrapper[T]) Ne(f Field[T], v any) *QueryWrapper[T] { return w.add(" <> ?", f, v) }
func (w *QueryWrapper[T]) Gt(f Field[T], v any) *QueryWrapper[T] { return w.add(" > ?", f, v) }
func (w *QueryWrapper[T]) Ge(f Field[T], v any) *QueryWrapper[T] { return w.add(" >= ?", f, v) }
func (w *QueryWrapper[T]) Lt(f Field[T], v any) *QueryWrapper[T] { return w.add(" < ?", f, v) }
func (w *QueryWrapper[T]) Le(f Field[T], v any) *QueryWrapper[T] { return w.add(" <= ?", f, v) }

func (w *QueryWrapper[T]) IsNull(f Field[T]) *QueryWrapper[T]    { return w.add(" IS NULL", f) }
func (w *QueryWrapper[T]) IsNotNull(f Field[T]) *QueryWrapper[T] { return w.add(" IS NOT NULL", f) }

func (w *QueryWrapper[T]) Like(f Field[T], kw string) *QueryWrapper[T] {
	return w.add(" LIKE ?", f, "%"+kw+"%")
}

func (w *QueryWrapper[T]) NotLike(f Field[T], kw string) *QueryWrapper[T] {
	return w.add(" NOT LIKE ?", f, "%"+kw+"%")
}

// LikeLeft 前缀匹配,如 LikeLeft(fName, "tom") → LIKE 'tom%'。
func (w *QueryWrapper[T]) LikeLeft(f Field[T], prefix string) *QueryWrapper[T] {
	return w.add(" LIKE ?", f, prefix+"%")
}

// LikeRight 后缀匹配,如 LikeRight(fName, "tom") → LIKE '%tom'。
func (w *QueryWrapper[T]) LikeRight(f Field[T], suffix string) *QueryWrapper[T] {
	return w.add(" LIKE ?", f, "%"+suffix)
}

func (w *QueryWrapper[T]) In(f Field[T], vals ...any) *QueryWrapper[T] {
	return w.addIn(" IN ", f, vals)
}

func (w *QueryWrapper[T]) NotIn(f Field[T], vals ...any) *QueryWrapper[T] {
	return w.addIn(" NOT IN ", f, vals)
}

func (w *QueryWrapper[T]) addIn(op string, f Field[T], vals []any) *QueryWrapper[T] {
	if len(vals) == 0 {
		panic("gorm_max: In/NotIn 需要至少一个值")
	}
	return w.add(op+placeholders(len(vals)), f, vals...)
}

func (w *QueryWrapper[T]) Between(f Field[T], a, b any) *QueryWrapper[T] {
	return w.add(" BETWEEN ? AND ?", f, a, b)
}

func (w *QueryWrapper[T]) NotBetween(f Field[T], a, b any) *QueryWrapper[T] {
	return w.add(" NOT BETWEEN ? AND ?", f, a, b)
}

// ---- 连接与嵌套 ----

// Or 下一个条件改用 OR 连接(MP 的 .or())。
func (w *QueryWrapper[T]) Or() *QueryWrapper[T] {
	w.pendingOr = true
	return w
}

// AndSub 嵌套 AND 分组。
func (w *QueryWrapper[T]) AndSub(build func(*QueryWrapper[T]) *QueryWrapper[T]) *QueryWrapper[T] {
	return w.addSub(logicAnd, build)
}

// OrSub 嵌套 OR 分组。
func (w *QueryWrapper[T]) OrSub(build func(*QueryWrapper[T]) *QueryWrapper[T]) *QueryWrapper[T] {
	return w.addSub(logicOr, build)
}

// NotSub 嵌套 NOT 分组:NOT (组内条件,默认 AND 连接):
//
//	w.NotSub(func(q *QueryWrapper[User]) *QueryWrapper[User] {
//		return q.Eq(fStatus, 0).Gt(fAge, 60)
//	}) // → NOT (status = 0 AND age > 60)
func (w *QueryWrapper[T]) NotSub(build func(*QueryWrapper[T]) *QueryWrapper[T]) *QueryWrapper[T] {
	sub := &QueryWrapper[T]{res: w.res, dialect: w.dialect}
	if build != nil {
		build(sub)
	}
	if len(sub.conds) == 0 { // 空分组直接忽略
		return w
	}
	w.conds = append(w.conds, cond[T]{logic: w.takeLogic(logicAnd), sub: sub, negate: true})
	return w
}

func (w *QueryWrapper[T]) addSub(l logic, build func(*QueryWrapper[T]) *QueryWrapper[T]) *QueryWrapper[T] {
	sub := &QueryWrapper[T]{res: w.res, dialect: w.dialect}
	if build != nil {
		build(sub)
	}
	if len(sub.conds) == 0 { // 空分组直接忽略
		return w
	}
	w.conds = append(w.conds, cond[T]{logic: w.takeLogic(l), sub: sub})
	return w
}

// ---- 投影 / 排序 / 逃逸口 ----

// Select 指定查询列,不调用则默认所有列。
func (w *QueryWrapper[T]) Select(fs ...Field[T]) *QueryWrapper[T] {
	for _, f := range fs {
		w.selects = append(w.selects, w.res.resolve(f))
	}
	return w
}

func (w *QueryWrapper[T]) OrderByAsc(f Field[T]) *QueryWrapper[T]  { return w.addOrder(f, "ASC") }
func (w *QueryWrapper[T]) OrderByDesc(f Field[T]) *QueryWrapper[T] { return w.addOrder(f, "DESC") }

// GroupBy 分组(列名经字段选择器解析与方言转义)。
// 注意:带 GROUP BY 的 wrapper 不要用于 SelectPage/SelectCount,分组下的计数语义会变。
func (w *QueryWrapper[T]) GroupBy(fs ...Field[T]) *QueryWrapper[T] {
	for _, f := range fs {
		w.groups = append(w.groups, w.res.resolve(f))
	}
	return w
}

// Having 手写 HAVING 片段(聚合表达式无法用字段选择器表达,对应 MP 的 having(sql, params))。
// sql 为开发者提供的片段,值一律走占位符:
//
//	w.GroupBy(fName).Having("COUNT(*) > ?", 1)
func (w *QueryWrapper[T]) Having(sql string, args ...any) *QueryWrapper[T] {
	w.havings = append(w.havings, havingClause{sql: sql, args: args})
	return w
}

// Apply 直接操作 *gorm.DB,处理 wrapper 覆盖不到的场景(join/锁/原生片段等)。
func (w *QueryWrapper[T]) Apply(fn func(*gorm.DB) *gorm.DB) *QueryWrapper[T] {
	w.applies = append(w.applies, fn)
	return w
}

// ---- JOIN / 去重 ----

// Joins 原生 JOIN 片段透传,值走占位符,可多次调用叠加;传关系字段名时行为同 gorm 的关联 Join:
//
//	w.Joins("LEFT JOIN test_profiles p ON p.id = test_users.profile_id")
//	w.Joins("INNER JOIN orders o ON o.user_id = test_users.id AND o.amount > ?", 100)
//	w.Joins("Profile") // 关联 Join
//
// 关联表的过滤条件可内联在 JOIN 片段里(如上),或用 Apply 追加。
func (w *QueryWrapper[T]) Joins(joinSQL string, args ...any) *QueryWrapper[T] {
	if joinSQL == "" {
		panic("gorm_max: Joins 片段不能为空")
	}
	w.joins = append(w.joins, joinClause{sql: joinSQL, args: args})
	return w
}

// Distinct 去重。无参:SELECT DISTINCT 全部列;有参:SELECT DISTINCT 指定列(字段选择器)。
// 与 Select 同用且有参时,列以 Distinct 为准。
// 注意:配合 SelectPage/SelectCount 时的计数语义以 gorm 行为为准,慎用(同 GroupBy)。
func (w *QueryWrapper[T]) Distinct(fs ...Field[T]) *QueryWrapper[T] {
	w.distinct = true
	for _, f := range fs {
		w.distinctCols = append(w.distinctCols, w.res.resolve(f))
	}
	return w
}

// ---- 关联预载 ----

// Preload 关联预载(透传 gorm),assoc 为模型的关系字段名(Go 字段名,非列名)。
// 支持嵌套与条件:
//
//	w.Preload("Profile")
//	w.Preload("Orders", "state <> ?", "cancelled")
//	w.Preload("Orders.Items")
//
// 同一关联多次调用时,后者的条件覆盖前者(gorm 的 Preloads 是 map)。
func (w *QueryWrapper[T]) Preload(assoc string, args ...any) *QueryWrapper[T] {
	if assoc == "" {
		panic("gorm_max: Preload 关联名不能为空")
	}
	w.preloads = append(w.preloads, preloadClause{assoc: assoc, args: args})
	return w
}

// ---- 行数截断 ----

// Limit 返回行数上限,独立于 SelectPage 的轻量截断(如"取最新 N 条"):
//
//	m.SelectList(ctx, m.NewQuery().OrderByDesc(fID).Limit(10))
//
// n 为负数 panic;0 合法(返回空)。与 SelectPage 同用时,分页参数覆盖 Limit/Offset。
func (w *QueryWrapper[T]) Limit(n int64) *QueryWrapper[T] {
	if n < 0 {
		panic("gorm_max: Limit 不能为负数")
	}
	w.limit = &n
	return w
}

// Offset 跳过的行数,一般与 Limit 同用(MySQL/SQLite 语法要求 OFFSET 必须跟在 LIMIT 后)。
func (w *QueryWrapper[T]) Offset(n int64) *QueryWrapper[T] {
	if n < 0 {
		panic("gorm_max: Offset 不能为负数")
	}
	w.offset = &n
	return w
}

// ---- 行锁 ----

// LockOption 行锁修饰项。
type LockOption string

const (
	// LockNoWait 锁冲突时不等待,立即报错。
	LockNoWait LockOption = "NOWAIT"
	// LockSkipLocked 跳过已被其他事务锁定的行。
	LockSkipLocked LockOption = "SKIP LOCKED"
)

// ForUpdate 行级排他锁(SELECT ... FOR UPDATE),必须在事务内使用才有意义。
// MySQL/PostgreSQL/SQL Server/Oracle 支持;SQLite 不支持(仅可验证 SQL 生成)。
// 不适用于带 GroupBy 的聚合查询(数据库会报错);配合 SelectPage 时 count 语句也会带锁。
func (w *QueryWrapper[T]) ForUpdate(opts ...LockOption) *QueryWrapper[T] {
	return w.addLock(clause.LockingStrengthUpdate, opts)
}

// ForShare 行级共享锁(SELECT ... FOR SHARE),语义同 ForUpdate,锁模式为共享。
func (w *QueryWrapper[T]) ForShare(opts ...LockOption) *QueryWrapper[T] {
	return w.addLock(clause.LockingStrengthShare, opts)
}

func (w *QueryWrapper[T]) addLock(strength string, opts []LockOption) *QueryWrapper[T] {
	lk := clause.Locking{Strength: strength}
	for _, o := range opts {
		if lk.Options != "" {
			lk.Options += " "
		}
		lk.Options += string(o)
	}
	w.locking = append(w.locking, lk)
	return w
}

// ---- 内部 ----

func (w *QueryWrapper[T]) add(op string, f Field[T], vals ...any) *QueryWrapper[T] {
	w.conds = append(w.conds, cond[T]{logic: w.takeLogic(logicAnd), op: op, col: w.res.resolve(f), vals: vals})
	return w
}

func (w *QueryWrapper[T]) takeLogic(def logic) logic {
	if w.pendingOr {
		w.pendingOr = false
		return logicOr
	}
	return def
}

func (w *QueryWrapper[T]) addOrder(f Field[T], dir string) *QueryWrapper[T] {
	w.orders = append(w.orders, order{col: w.res.resolve(f), dir: dir})
	return w
}

func (w *QueryWrapper[T]) buildSQL() (string, []any) {
	if len(w.conds) == 0 {
		return "", nil
	}
	var sb strings.Builder
	var args []any
	w.writeConds(&sb, &args, w.conds)
	return sb.String(), args
}

func (w *QueryWrapper[T]) writeConds(sb *strings.Builder, args *[]any, conds []cond[T]) {
	for i, c := range conds {
		if i > 0 {
			if c.logic == logicOr {
				sb.WriteString(" OR ")
			} else {
				sb.WriteString(" AND ")
			}
		}
		if c.sub != nil {
			if c.negate {
				sb.WriteString("NOT ")
			}
			sb.WriteString("(")
			w.writeConds(sb, args, c.sub.conds)
			sb.WriteString(")")
			continue
		}
		w.writeCol(sb, w.colName(c.col))
		sb.WriteString(c.op)
		*args = append(*args, c.vals...)
	}
}

// colName 无 JOIN 时用裸列名;有 JOIN 时带表名限定,防多表同名列歧义。
func (w *QueryWrapper[T]) colName(c *Column) string {
	if len(w.joins) > 0 && w.res.table != "" {
		return w.res.table + "." + c.Name
	}
	return c.Name
}

// writeCol 列名经方言转义(反引号/双引号),防保留字与注入。
func (w *QueryWrapper[T]) writeCol(sb *strings.Builder, name string) {
	if w.dialect != nil {
		w.dialect.QuoteTo(sb, name)
	} else {
		sb.WriteString(name)
	}
}

func (w *QueryWrapper[T]) buildSelectSQL() string {
	if len(w.selects) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, c := range w.selects {
		if i > 0 {
			sb.WriteString(", ")
		}
		w.writeCol(&sb, w.colName(c))
	}
	return sb.String()
}

// buildSelectWithDistinct 组装 SELECT 投影(含 DISTINCT 标志),列名经方言转义;
// 整体作为裸片段交给 gorm 的 Select 渲染。
func (w *QueryWrapper[T]) buildSelectWithDistinct() string {
	sel := w.buildSelectSQL()
	if !w.distinct {
		return sel
	}
	var sb strings.Builder
	sb.WriteString("DISTINCT ")
	if len(w.distinctCols) > 0 {
		for i, c := range w.distinctCols {
			if i > 0 {
				sb.WriteString(", ")
			}
			w.writeCol(&sb, w.colName(c))
		}
	} else if sel != "" {
		sb.WriteString(sel)
	} else {
		sb.WriteString("*")
	}
	return sb.String()
}

func (w *QueryWrapper[T]) buildGroupSQL() string {
	if len(w.groups) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, c := range w.groups {
		if i > 0 {
			sb.WriteString(", ")
		}
		w.writeCol(&sb, w.colName(c))
	}
	return sb.String()
}

// buildHavingSQL 多个片段用 AND 连接,各自加括号防优先级问题。
func (w *QueryWrapper[T]) buildHavingSQL() (string, []any) {
	if len(w.havings) == 0 {
		return "", nil
	}
	var sb strings.Builder
	var args []any
	for i, h := range w.havings {
		if i > 0 {
			sb.WriteString(" AND ")
		}
		sb.WriteString("(")
		sb.WriteString(h.sql)
		sb.WriteString(")")
		args = append(args, h.args...)
	}
	return sb.String(), args
}

func (w *QueryWrapper[T]) buildOrderSQL() []string {
	if len(w.orders) == 0 {
		return nil
	}
	out := make([]string, 0, len(w.orders))
	for _, o := range w.orders {
		var sb strings.Builder
		w.writeCol(&sb, w.colName(o.col))
		sb.WriteString(" ")
		sb.WriteString(o.dir)
		out = append(out, sb.String())
	}
	return out
}

// ---- Clone ----

// Clone 深拷贝 wrapper(含嵌套分组),用于复用基础条件:
//
//	base := m.NewQuery().Eq(fStatus, 1)
//	w := base.Clone().Like(fName, "tom") // base 不受影响
//
// resolver 为不可变共享;克隆是纯数据复制,可安全并发使用(各自独立)。
func (w *QueryWrapper[T]) Clone() *QueryWrapper[T] {
	clone := &QueryWrapper[T]{
		res:          w.res,
		dialect:      w.dialect,
		distinct:     w.distinct,
		distinctCols: cloneSlice(w.distinctCols),
		selects:      cloneSlice(w.selects),
		orders:       cloneSlice(w.orders),
		groups:       cloneSlice(w.groups),
		havings:      cloneSlice(w.havings),
		preloads:     cloneSlice(w.preloads),
		joins:        cloneSlice(w.joins),
		locking:      cloneSlice(w.locking),
		applies:      cloneSlice(w.applies),
		pendingOr:    w.pendingOr,
	}
	if len(w.conds) > 0 {
		clone.conds = make([]cond[T], len(w.conds))
		for i, c := range w.conds {
			clone.conds[i] = c.clone() // 嵌套分组递归深拷贝
		}
	}
	return clone
}

// ---- 类型化字段接入口(M5 字段体系,与闭包 API 可混用) ----

// Where 挂载公开条件节点,顶层平铺彼此 AND,分组用 And/Or/Not 组合:
//
//	w.Where(f.Name.Like("tom"), f.Age.Gt(10), Or(f.Age.Lt(5), f.ID.Gt(100)))
//
// Or() 开关后的第一个 Where 条件同样以 OR 接入(与闭包方法语义一致)。
func (w *QueryWrapper[T]) Where(conds ...Cond) *QueryWrapper[T] {
	for i, c := range conds {
		ic := condFrom[T](c)
		if i == 0 && w.pendingOr {
			ic.logic = logicOr
			w.pendingOr = false
		}
		w.conds = append(w.conds, ic)
	}
	return w
}

// Order 挂载排序项:f.ID.Desc(), f.Name.Asc()。
func (w *QueryWrapper[T]) Order(ords ...Order) *QueryWrapper[T] {
	for _, o := range ords {
		dir := "ASC"
		if o.desc {
			dir = "DESC"
		}
		w.orders = append(w.orders, order{col: &Column{Name: o.col}, dir: dir})
	}
	return w
}

// SelectF 按字段句柄指定投影列(闭包写法用 Select)。
func (w *QueryWrapper[T]) SelectF(fs ...FieldRef) *QueryWrapper[T] {
	for _, f := range fs {
		w.selects = append(w.selects, &Column{Name: f.Column()})
	}
	return w
}

// condFrom 把公开条件节点递归转换为内部渲染结构。
func condFrom[T any](c Cond) cond[T] {
	if len(c.sub) > 0 {
		sub := &QueryWrapper[T]{}
		for i, sc := range c.sub {
			ic := condFrom[T](sc)
			if i > 0 && c.join == logicOr {
				ic.logic = logicOr
			}
			sub.conds = append(sub.conds, ic)
		}
		return cond[T]{sub: sub, negate: c.negate}
	}
	return cond[T]{op: c.op, col: &Column{Name: c.col}, vals: c.vals}
}

// ---- UpdateWrapper:SET + WHERE ----

type setClause struct {
	col  string
	val  any
	expr string // 非空则走 gorm.Expr(裸片段,由开发者保证语法)
	args []any
	incr *incrAction // 非空则按列自引用表达式渲染(列名执行时经方言转义)
}

// incrAction 自增/自减动作,由 Incr/Decr 使用。
type incrAction struct {
	neg bool // true = 自减
	n   int64
}

// UpdateWrapper 更新条件构造器(条件方法与 QueryWrapper 一致,返回自身类型)。
type UpdateWrapper[T any] struct {
	QueryWrapper[T]
	sets []setClause
}

// NewUpdate 独立创建;在 Mapper/Service 上请用 NewUpdate 方法。
func NewUpdate[T any]() *UpdateWrapper[T] {
	return &UpdateWrapper[T]{QueryWrapper: *NewQuery[T]()}
}

func (w *UpdateWrapper[T]) Set(f Field[T], v any) *UpdateWrapper[T] {
	c := w.res.resolve(f)
	w.sets = append(w.sets, setClause{col: c.Name, val: v})
	return w
}

// SetExpr 表达式赋值,如 w.SetExpr(fAge, "age + ?", 1)。
func (w *UpdateWrapper[T]) SetExpr(f Field[T], expr string, args ...any) *UpdateWrapper[T] {
	c := w.res.resolve(f)
	w.sets = append(w.sets, setClause{col: c.Name, expr: expr, args: args})
	return w
}

// Incr 自增:SET col = col + n。列名来自字段选择器并在执行时经方言转义,
// 无需手写列名(保留字列安全)。仅整型列;浮点用 SetExpr。
func (w *UpdateWrapper[T]) Incr(f Field[T], n int64) *UpdateWrapper[T] {
	if n <= 0 {
		panic("gorm_max: Incr 的 n 必须为正数(自减请用 Decr)")
	}
	c := w.res.resolve(f)
	w.sets = append(w.sets, setClause{col: c.Name, incr: &incrAction{n: n}})
	return w
}

// Decr 自减:SET col = col - n。扣减到负数是被允许的;防负数请叠加自身条件:
//
//	m.Update(ctx, uw.Decr(fStock, 1).Ge(fStock, 1)) // 条件不满足时 0 行
func (w *UpdateWrapper[T]) Decr(f Field[T], n int64) *UpdateWrapper[T] {
	if n <= 0 {
		panic("gorm_max: Decr 的 n 必须为正数(自增请用 Incr)")
	}
	c := w.res.resolve(f)
	w.sets = append(w.sets, setClause{col: c.Name, incr: &incrAction{neg: true, n: n}})
	return w
}

func (w *UpdateWrapper[T]) setsMap() map[string]any {
	if len(w.sets) == 0 {
		return nil
	}
	m := make(map[string]any, len(w.sets))
	for _, s := range w.sets {
		switch {
		case s.incr != nil: // 列自引用:SET col = col ± n,列名经方言转义(保留字安全)
			op := " + "
			if s.incr.neg {
				op = " - "
			}
			var sb strings.Builder
			w.writeCol(&sb, s.col)
			sb.WriteString(op)
			sb.WriteString("?")
			m[s.col] = gorm.Expr(sb.String(), s.incr.n)
		case s.expr != "":
			m[s.col] = gorm.Expr(s.expr, s.args...)
		default:
			m[s.col] = s.val
		}
	}
	return m
}

// 条件方法重声明,保持链式返回 *UpdateWrapper[T](Go 方法不能带泛型参数,只能这样桥接)。

func (w *UpdateWrapper[T]) Eq(f Field[T], v any) *UpdateWrapper[T] {
	w.QueryWrapper.Eq(f, v)
	return w
}

func (w *UpdateWrapper[T]) Ne(f Field[T], v any) *UpdateWrapper[T] {
	w.QueryWrapper.Ne(f, v)
	return w
}

func (w *UpdateWrapper[T]) Gt(f Field[T], v any) *UpdateWrapper[T] {
	w.QueryWrapper.Gt(f, v)
	return w
}

func (w *UpdateWrapper[T]) Ge(f Field[T], v any) *UpdateWrapper[T] {
	w.QueryWrapper.Ge(f, v)
	return w
}

func (w *UpdateWrapper[T]) Lt(f Field[T], v any) *UpdateWrapper[T] {
	w.QueryWrapper.Lt(f, v)
	return w
}

func (w *UpdateWrapper[T]) Le(f Field[T], v any) *UpdateWrapper[T] {
	w.QueryWrapper.Le(f, v)
	return w
}

func (w *UpdateWrapper[T]) Like(f Field[T], kw string) *UpdateWrapper[T] {
	w.QueryWrapper.Like(f, kw)
	return w
}

func (w *UpdateWrapper[T]) NotLike(f Field[T], kw string) *UpdateWrapper[T] {
	w.QueryWrapper.NotLike(f, kw)
	return w
}

func (w *UpdateWrapper[T]) In(f Field[T], vals ...any) *UpdateWrapper[T] {
	w.QueryWrapper.In(f, vals...)
	return w
}

func (w *UpdateWrapper[T]) NotIn(f Field[T], vals ...any) *UpdateWrapper[T] {
	w.QueryWrapper.NotIn(f, vals...)
	return w
}

func (w *UpdateWrapper[T]) Between(f Field[T], a, b any) *UpdateWrapper[T] {
	w.QueryWrapper.Between(f, a, b)
	return w
}

func (w *UpdateWrapper[T]) NotBetween(f Field[T], a, b any) *UpdateWrapper[T] {
	w.QueryWrapper.NotBetween(f, a, b)
	return w
}

func (w *UpdateWrapper[T]) IsNull(f Field[T]) *UpdateWrapper[T] {
	w.QueryWrapper.IsNull(f)
	return w
}

func (w *UpdateWrapper[T]) IsNotNull(f Field[T]) *UpdateWrapper[T] {
	w.QueryWrapper.IsNotNull(f)
	return w
}

// Or 下一个条件改用 OR 连接。
func (w *UpdateWrapper[T]) Or() *UpdateWrapper[T] {
	w.QueryWrapper.Or()
	return w
}

// AndSub 嵌套 AND 分组。
func (w *UpdateWrapper[T]) AndSub(build func(*QueryWrapper[T]) *QueryWrapper[T]) *UpdateWrapper[T] {
	w.QueryWrapper.AndSub(build)
	return w
}

// OrSub 嵌套 OR 分组。
func (w *UpdateWrapper[T]) OrSub(build func(*QueryWrapper[T]) *QueryWrapper[T]) *UpdateWrapper[T] {
	w.QueryWrapper.OrSub(build)
	return w
}

// NotSub 嵌套 NOT 分组。
func (w *UpdateWrapper[T]) NotSub(build func(*QueryWrapper[T]) *QueryWrapper[T]) *UpdateWrapper[T] {
	w.QueryWrapper.NotSub(build)
	return w
}

// Clone 深拷贝更新构造器(含 SET 与条件,语义同 QueryWrapper.Clone)。
func (w *UpdateWrapper[T]) Clone() *UpdateWrapper[T] {
	return &UpdateWrapper[T]{
		QueryWrapper: *w.QueryWrapper.Clone(),
		sets:         cloneSlice(w.sets),
	}
}

// Where 挂载公开条件节点(语义同 QueryWrapper.Where)。
func (w *UpdateWrapper[T]) Where(conds ...Cond) *UpdateWrapper[T] {
	w.QueryWrapper.Where(conds...)
	return w
}

// SetItems 挂载类型化赋值项,与 Set/SetExpr/Incr/Decr 可混用:
//
//	uw.SetItems(f.NickName.SetTo("n"), f.Age.Incr(1))
func (w *UpdateWrapper[T]) SetItems(items ...SetItem) *UpdateWrapper[T] {
	for _, it := range items {
		w.sets = append(w.sets, setClause{col: it.col, val: it.val, incr: it.incr})
	}
	return w
}

// ---- 编译期类型校验的函数式写法(可选增强) ----
// V 由闭包返回类型推断,值类型不匹配直接编译报错:
//
//	gorm_max.Eq(w, func(u *User) string { return u.Name }, "tom")

func Eq[T any, V any](w *QueryWrapper[T], sel func(*T) V, v V) *QueryWrapper[T] {
	return w.add(" = ?", func(u *T) any { return sel(u) }, v)
}

func Ne[T any, V any](w *QueryWrapper[T], sel func(*T) V, v V) *QueryWrapper[T] {
	return w.add(" <> ?", func(u *T) any { return sel(u) }, v)
}

func Gt[T any, V any](w *QueryWrapper[T], sel func(*T) V, v V) *QueryWrapper[T] {
	return w.add(" > ?", func(u *T) any { return sel(u) }, v)
}

func Ge[T any, V any](w *QueryWrapper[T], sel func(*T) V, v V) *QueryWrapper[T] {
	return w.add(" >= ?", func(u *T) any { return sel(u) }, v)
}

func Lt[T any, V any](w *QueryWrapper[T], sel func(*T) V, v V) *QueryWrapper[T] {
	return w.add(" < ?", func(u *T) any { return sel(u) }, v)
}

func Le[T any, V any](w *QueryWrapper[T], sel func(*T) V, v V) *QueryWrapper[T] {
	return w.add(" <= ?", func(u *T) any { return sel(u) }, v)
}

func Like[T any, V any](w *QueryWrapper[T], sel func(*T) V, kw string) *QueryWrapper[T] {
	return w.add(" LIKE ?", func(u *T) any { return sel(u) }, "%"+kw+"%")
}

func In[T any, V any](w *QueryWrapper[T], sel func(*T) V, vals ...V) *QueryWrapper[T] {
	anyVals := make([]any, len(vals))
	for i, v := range vals {
		anyVals[i] = v
	}
	return w.In(func(u *T) any { return sel(u) }, anyVals...)
}
