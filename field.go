package gorm_max

import (
	"strings"
	"time"
)

// ---- 字段句柄 ----

// FieldRef 类型化字段句柄的公共接口。M5b 生成器为每个模型产出字段句柄集,
// 手写字段集同样使用本文件的类型与构造器。
type FieldRef interface {
	Column() string
}

// TypedField 类型化字段句柄:列名 + 值类型 V。
// 方法产生公开条件节点(Cond)、赋值项(SetItem)与排序项(Order),
// 值类型在编译期校验(如 IntField.Gt("abc") 无法编译)。
type TypedField[V any] struct {
	col string
}

func (f TypedField[V]) Column() string { return f.col }

// ---- 条件方法 ----

func (f TypedField[V]) cond(op string, vals ...any) Cond {
	return Cond{op: op, col: f.col, vals: vals}
}

func (f TypedField[V]) Eq(v V) Cond { return f.cond(" = ?", v) }
func (f TypedField[V]) Ne(v V) Cond { return f.cond(" <> ?", v) }
func (f TypedField[V]) Gt(v V) Cond { return f.cond(" > ?", v) }
func (f TypedField[V]) Ge(v V) Cond { return f.cond(" >= ?", v) }
func (f TypedField[V]) Lt(v V) Cond { return f.cond(" < ?", v) }
func (f TypedField[V]) Le(v V) Cond { return f.cond(" <= ?", v) }

func (f TypedField[V]) IsNull() Cond    { return f.cond(" IS NULL") }
func (f TypedField[V]) IsNotNull() Cond { return f.cond(" IS NOT NULL") }

func (f TypedField[V]) In(vals ...V) Cond {
	if len(vals) == 0 {
		panic("gorm_max: In/NotIn 需要至少一个值")
	}
	return f.cond(" IN "+placeholders(len(vals)), anySlice(vals)...)
}

func (f TypedField[V]) NotIn(vals ...V) Cond {
	if len(vals) == 0 {
		panic("gorm_max: In/NotIn 需要至少一个值")
	}
	return f.cond(" NOT IN "+placeholders(len(vals)), anySlice(vals)...)
}

func (f TypedField[V]) Between(a, b V) Cond    { return f.cond(" BETWEEN ? AND ?", a, b) }
func (f TypedField[V]) NotBetween(a, b V) Cond { return f.cond(" NOT BETWEEN ? AND ?", a, b) }

// ---- 赋值方法(经 UpdateWrapper.SetItems 挂载) ----

// SetTo 赋值:SET col = v。
func (f TypedField[V]) SetTo(v V) SetItem { return SetItem{col: f.col, val: v} }

// Incr 自增:SET col = col + n(列名经方言转义,保留字安全)。仅整型列。
func (f TypedField[V]) Incr(n int64) SetItem {
	if n <= 0 {
		panic("gorm_max: Incr 的 n 必须为正数(自减请用 Decr)")
	}
	return SetItem{col: f.col, incr: &incrAction{n: n}}
}

// Decr 自减:SET col = col - n。防负数请在 Where 里叠加自身条件。
func (f TypedField[V]) Decr(n int64) SetItem {
	if n <= 0 {
		panic("gorm_max: Decr 的 n 必须为正数(自增请用 Incr)")
	}
	return SetItem{col: f.col, incr: &incrAction{neg: true, n: n}}
}

// ---- 排序 ----

func (f TypedField[V]) Asc() Order  { return Order{col: f.col} }
func (f TypedField[V]) Desc() Order { return Order{col: f.col, desc: true} }

// ---- 具体字段类型与构造器 ----
// M5b 生成器按模型列的 Go 类型选择字段类型;类型为定义类型(非别名),
// 以便 StringField 等可以携带专属方法(Like 系)而不污染其他类型。

type (
	StringField struct{ TypedField[string] }
	IntField    struct{ TypedField[int] }
	Int8Field   struct{ TypedField[int8] }
	Int16Field  struct{ TypedField[int16] }
	Int32Field  struct{ TypedField[int32] }
	Int64Field  struct{ TypedField[int64] }

	UintField   struct{ TypedField[uint] }
	Uint8Field  struct{ TypedField[uint8] }
	Uint16Field struct{ TypedField[uint16] }
	Uint32Field struct{ TypedField[uint32] }
	Uint64Field struct{ TypedField[uint64] }

	Float32Field struct{ TypedField[float32] }
	Float64Field struct{ TypedField[float64] }
	BoolField    struct{ TypedField[bool] }
	TimeField    struct{ TypedField[time.Time] }
	BytesField   struct{ TypedField[[]byte] }
	GenericField struct{ TypedField[any] } // 兜底:自定义类型/序列化字段
)

func NewString(col string) StringField   { return StringField{TypedField[string]{col}} }
func NewInt(col string) IntField         { return IntField{TypedField[int]{col}} }
func NewInt8(col string) Int8Field       { return Int8Field{TypedField[int8]{col}} }
func NewInt16(col string) Int16Field     { return Int16Field{TypedField[int16]{col}} }
func NewInt32(col string) Int32Field     { return Int32Field{TypedField[int32]{col}} }
func NewInt64(col string) Int64Field     { return Int64Field{TypedField[int64]{col}} }
func NewUint(col string) UintField       { return UintField{TypedField[uint]{col}} }
func NewUint8(col string) Uint8Field     { return Uint8Field{TypedField[uint8]{col}} }
func NewUint16(col string) Uint16Field   { return Uint16Field{TypedField[uint16]{col}} }
func NewUint32(col string) Uint32Field   { return Uint32Field{TypedField[uint32]{col}} }
func NewUint64(col string) Uint64Field   { return Uint64Field{TypedField[uint64]{col}} }
func NewFloat32(col string) Float32Field { return Float32Field{TypedField[float32]{col}} }
func NewFloat64(col string) Float64Field { return Float64Field{TypedField[float64]{col}} }
func NewBool(col string) BoolField       { return BoolField{TypedField[bool]{col}} }
func NewTime(col string) TimeField       { return TimeField{TypedField[time.Time]{col}} }
func NewBytes(col string) BytesField     { return BytesField{TypedField[[]byte]{col}} }
func NewAny(col string) GenericField     { return GenericField{TypedField[any]{col}} }

// NewTyped 任意自定义类型的字段句柄构造器(生成器对非预声明类型使用,精确到原类型):
//
//	var Status = gorm_max.NewTyped[model.Status]("status")
func NewTyped[V any](col string) TypedField[V] { return TypedField[V]{col: col} }

// Like 系:仅字符串字段携带。

func (f StringField) Like(kw string) Cond          { return f.cond(" LIKE ?", "%"+kw+"%") }
func (f StringField) NotLike(kw string) Cond       { return f.cond(" NOT LIKE ?", "%"+kw+"%") }
func (f StringField) LikeLeft(prefix string) Cond  { return f.cond(" LIKE ?", prefix+"%") }
func (f StringField) LikeRight(suffix string) Cond { return f.cond(" LIKE ?", "%"+suffix) }

// ---- 公开条件节点与组合子 ----

// Cond 条件节点:字段方法产生叶子,And/Or/Not 组合分组,经 wrapper.Where 挂载。
// 顶层平铺条件彼此 AND 连接;OR 接入的分组继续用 wrapper 的 OrSub(可与 Where 混用)。
type Cond struct {
	op     string
	col    string
	vals   []any
	join   logic // 分组内子条件的连接方式(Or 组为 OR,其余 AND)
	negate bool
	sub    []Cond
}

// Or 组内 OR 连接、整体 AND 接入:a AND (b OR c)。
func Or(conds ...Cond) Cond { return groupCond(logicOr, false, conds) }

// And 显式分组(组内 AND)。平铺时与直接传入 Where 等价;深层嵌套时用于表达结构。
func And(conds ...Cond) Cond { return groupCond(logicAnd, false, conds) }

// Not 组内 AND 连接并取反:NOT (a AND b)。
func Not(conds ...Cond) Cond { return groupCond(logicAnd, true, conds) }

func groupCond(join logic, negate bool, conds []Cond) Cond {
	if len(conds) == 0 {
		panic("gorm_max: 条件组不能为空")
	}
	return Cond{join: join, negate: negate, sub: conds}
}

// ---- 赋值项与排序项 ----

// SetItem 类型化更新赋值项,由字段方法 SetTo/Incr/Decr 产生,经 UpdateWrapper.SetItems 挂载。
type SetItem struct {
	col  string
	val  any
	incr *incrAction
}

// Order 排序项,由字段方法 Asc/Desc 产生,经 wrapper.Order 挂载。
type Order struct {
	col  string
	desc bool
}

// ---- 工具 ----

func placeholders(n int) string {
	return "(" + strings.TrimSuffix(strings.Repeat("?,", n), ",") + ")"
}

func anySlice[E any](s []E) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}
