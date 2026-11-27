# 原子 upsert

数据库级"插入或更新":唯一键冲突时按策略更新或忽略,**单条 SQL 原子完成,无应用层竞态**——相比 `SaveOrUpdate`(先查主键再插/改),高并发写场景必须用这一组。

方言行为(已核实 gorm 源码):

| 方言 | 更新策略 | 忽略策略 |
|---|---|---|
| MySQL | `ON DUPLICATE KEY UPDATE col=VALUES(col)` | 自动降级为 `pk=pk` 的 no-op 更新 |
| PostgreSQL / SQLite | `ON CONFLICT (cols) DO UPDATE SET col=excluded.col` | `ON CONFLICT DO NOTHING` |

## InsertOnConflict

```go
func (m *Mapper[T]) InsertOnConflict(ctx context.Context, entity *T, act ConflictAction) (int64, error)
```

单条原子 upsert。

**返回**:受影响行数——新插入 1;冲突更新 1;冲突忽略 0。

**注意**:冲突走更新分支时,**自增主键不会回填到实体**(该行本已存在)。

```go
// 冲突按 name 唯一键,只更新 age
n, err := m.InsertOnConflict(ctx,
	&User{Name: "tom", Email: "new@x.com", Age: 2},
	gorm_max.ConflictAction{
		Columns:      []string{"name"},
		UpdateFields: []any{func(x *User) any { return &x.Age }},
	})
```

## InsertBatchOnConflict

```go
func (m *Mapper[T]) InsertBatchOnConflict(ctx context.Context, entities []*T, act ConflictAction, batchSize ...int) (int64, error)
```

批量 upsert,语义同上,按 batchSize 分批(默认 500)。空切片返回 (0, nil)。

```go
n, err := m.InsertBatchOnConflict(ctx, []*User{
	{Name: "tom", Age: 9},  // 冲突 → 更新
	{Name: "jerry", Age: 1}, // 新插入
}, gorm_max.ConflictAction{Columns: []string{"name"}, UpdateAll: true})
```

> 受影响行数在不同方言口径不同(MySQL 默认统计下"更新"计 2),**业务判断请以查询结果为准**,不要依赖返回值精确值。

## ConflictAction

```go
type ConflictAction struct {
	Columns      []string // 冲突判定列
	UpdateFields []any    // 冲突时更新的列(字段选择器)
	UpdateAll    bool     // 冲突时全列覆盖
	Ignore       bool     // 冲突时忽略
}
```

| 字段 | 说明 |
|---|---|
| `Columns` | 冲突判定列。仅 PostgreSQL/SQLite 支持;**MySQL 忽略此设置**(按任意唯一键自动判定) |
| `UpdateFields` | 冲突时需要更新的列,字段选择器形式,只覆盖指定列、其余保留 |
| `UpdateAll` | 冲突时更新除主键、自动创建时间外的全部列(UpdatedAt 自动刷新)。**未指定 Columns 时 gorm 默认以主键作冲突判定列**——唯一键不是主键时必须显式指定 Columns |
| `Ignore` | 冲突时忽略本次插入。**零值 ConflictAction 等价于 Ignore**(最安全默认) |

互斥规则:`Ignore` / `UpdateAll` / `UpdateFields` 至多选一,违反时返回 `ErrConflictActionInvalid`(构造阶段即报错,不会执行 SQL)。

典型用法对照:

```go
// 1. INSERT IGNORE 语义(零值即可)
m.InsertOnConflict(ctx, u, gorm_max.ConflictAction{})

// 2. 冲突时全列覆盖(按主键冲突,Columns 可省略)
m.InsertOnConflict(ctx, u, gorm_max.ConflictAction{UpdateAll: true})

// 3. 冲突时全列覆盖,唯一键非主键 → 必须指定 Columns
m.InsertOnConflict(ctx, u, gorm_max.ConflictAction{Columns: []string{"name"}, UpdateAll: true})

// 4. 冲突时只更新指定列(其余保留)
m.InsertOnConflict(ctx, u, gorm_max.ConflictAction{
	Columns:      []string{"name"},
	UpdateFields: []any{func(x *User) any { return &x.Age }},
})
```

## ErrConflictActionInvalid

```go
var ErrConflictActionInvalid = errors.New("gorm_max: ConflictAction 配置冲突(Ignore/UpdateAll/UpdateFields 至多选一)")
```

用 `errors.Is` 判断。
