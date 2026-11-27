# gorm-max 项目接入操作手册

以 [Memory-Record](../../memory-record)(Gin + GORM + 分层架构)为例,演示如何把 gorm-max 引入一个**已有**的分层项目。全新项目同理,只是顺序更简单。

适用项目形态:

```
项目根/
├── main.go
├── internal/
│   ├── core/initializer/   # mysql.go:创建 *gorm.DB,挂到 global.Db
│   ├── global/             # 全局单例
│   ├── models/entity/      # 实体(BaseTM 嵌入、TableName、雪花主键)
│   ├── repositories/       # 数据访问层
│   ├── services/           # 业务层
│   └── handler/            # Gin 处理器
└── go.mod
```

---

## 一、依赖引入

### 1. 引入库本体

`gorm-max` 只依赖 `gorm.io/gorm`(v2 全系兼容,不锁版本)。Memory-Record 当前使用 gorm v1.31.2,引入后 MVS 自动沿用你的版本,不会降级。

**发布前(本地路径 replace,推荐)**:在 `Memory-Record/go.mod` 中追加:

```go
require github.com/Lookfukc/gorm-max v0.0.0

replace github.com/Lookfukc/gorm-max => ../gorm-max
```

(两仓库同在 `D:\project\` 下,`../gorm-max` 即 `D:\project\gorm-max`。)

然后:

```bash
cd D:\project\memory-record
go mod tidy
```

**发布后(GitHub)**:

```bash
go get github.com/Lookfukc/gorm-max@latest
```

> 引入后新增依赖只有 `gorm-max` 本身;`gorm.io/gorm`、mysql 驱动项目里已有,不会引入新版本。

### 2. (可选)软删兼容插件

如果项目像 Memory-Record 一样使用 `is_deleted` 整型列做软删,建议引入 gorm 官方插件(见下文"实体改造"):

```bash
go get gorm.io/plugin/soft_delete
```

---

## 二、初始化注册

在 `internal/core/initializer/mysql.go` 的 `MysqlInit()` 里,`gorm.Open` + `AutoMigrate` 完成后追加一行注册(乐观锁/多租户按需):

```go
import (
	"github.com/Lookfukc/gorm-max"
	"github.com/Lookfukc/gorm-max/plugin"
	"Memory-Record/internal/models/entity"
)

func MysqlInit() error {
	// ...(原有逻辑:open、建库、连接、连接池、AutoMigrate 不变)...

	// === gorm-max 插件注册(按需选配) ===
	// 乐观锁:实体字段加 gorm:"version" 标签后自动生效
	_ = plugin.RegisterOptimisticLock(global.Db)
	// 多租户:多租户项目才需要(单租户应用跳过)
	// _ = plugin.RegisterTenant(global.Db, plugin.WithSkipTables("user"))

	// === gorm-max Mapper/Service 挂到 global(与 global.Db 并列,不改动原有变量) ===
	global.UserMapper = gorm_max.NewMapper[entity.User](global.Db)

	return nil
}
```

`global.go` 增加变量:

```go
import gorm_max "github.com/Lookfukc/gorm-max"

var (
	// ...(原有变量不变)...
	UserMapper *gorm_max.Mapper[entity.User]
)
```

> 现有代码继续用 `global.Db` 完全不受影响——gorm-max 是增量接入,不是替换。

---

## 三、实体改造

### 1. 软删:`is_deleted` 列零迁移兼容

Memory-Record 的 `BaseTM` 用 `IsDeleted *int` 做软删,**每个 repository 手写 `is_deleted = 0`**。建议换成 gorm 官方 soft_delete 插件的 `DeletedAt`(uint 整型)——其默认语义(`0=正常,1=删除`)与现有列**完全一致,零数据迁移**:

```go
// pkg/common/common.go — BaseTM 改造
import "gorm.io/plugin/soft_delete"

type BaseTM struct {
	CreatedAt  LocalDate `gorm:"column:create_time;type:datetime;default:CURRENT_TIMESTAMP;comment:创建时间" json:"createTime" swaggertype:"string"`
	UpdatedAt  LocalDate `gorm:"column:update_time;type:datetime;default:CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP;comment:修改时间" json:"updateTime" swaggertype:"string"`
	IsDeleted  soft_delete.DeletedAt `gorm:"column:is_deleted" json:"isDeleted"` // 原 *int 换成这个
	DeleteTime LocalDate `gorm:"column:delete_time;comment:注销时间" json:"deleteTime"`
}
```

效果:所有查询(包括 gorm-max 的全部查询、`SelectPage` 的 count)**自动追加 `is_deleted = 0`**,删除自动变 `UPDATE is_deleted = 1`;`hard_delete_repo.go` 里的 `Unscoped()` 用法照旧可用。

> 注意:`delete_time`(注销时间)不在插件职责内,仍由业务层在注销逻辑里维护;原有手写 `is_deleted = 0` 条件可以删除(重复条件不影响正确性,只影响可读性)。

> 注意:嵌入结构体的**类型名必须导出**(`BaseTM` ✓),gorm 才会解析嵌入字段。

### 2. 乐观锁(按需)

需要版本控制的实体加一个字段即可:

```go
type ChapterShare struct {
	// ...
	Version int `gorm:"column:version;default:1;comment:乐观锁版本" json:"version"`
	common.BaseTM
}
```

插入时给初始版本 1(或建表默认值);之后 `UpdateByID` 自动变为 `SET version=旧+1 ... WHERE id=? AND version=旧`,受影响行数为 0 即冲突,由业务层处理重试。

### 3. 主键

雪花主键(`uint64` + 雪花回调回填)无需任何改动,gorm-max 的主键检查/回填对 `uint64` 完全兼容。

---

## 四、Repository 改造(Before / After)

以 `internal/repositories/user_repo.go` 为例,只动数据访问层,方法签名不变,service/handler 零改动。

**Before(现状)**:

```go
func (r *UserRepository) FirstByID(ctx context.Context, id uint64) (*entity.User, error) {
	var user entity.User
	err := r.db.WithContext(ctx).
		Where("id = ? AND is_deleted = 0", id). // 手写软删条件、手写列名
		First(&user).Error
	return &user, err
}

func (r *UserRepository) ExistsByUserName(ctx context.Context, userName string) (bool, error) {
	var cnt int64
	err := r.db.WithContext(ctx).Model(&entity.User{}).
		Where("user_name = ? AND is_deleted = 0", userName).
		Count(&cnt).Error
	return cnt > 0, err
}
```

**After(gorm-max)**:

```go
import gorm_max "github.com/Lookfukc/gorm-max"

type UserRepository struct {
	m *gorm_max.Mapper[entity.User] // 替换 db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{m: gorm_max.NewMapper[entity.User](db)}
}

// FirstByID 根据ID查询当条用户(软删条件由 DeletedAt 字段自动追加)
func (r *UserRepository) FirstByID(ctx context.Context, id uint64) (*entity.User, error) {
	return r.m.SelectByID(ctx, id)
}

// FirstByUserName 根据用户名查询
func (r *UserRepository) FirstByUserName(ctx context.Context, userName string) (*entity.User, error) {
	return r.m.SelectOne(ctx, r.m.NewQuery().
		Eq(func(u *entity.User) any { return &u.UserName }, userName))
}

// ExistsByUserName 用户名是否已存在,注册查重
func (r *UserRepository) ExistsByUserName(ctx context.Context, userName string) (bool, error) {
	return r.m.Exists(ctx, r.m.NewQuery().
		Eq(func(u *entity.User) any { return &u.UserName }, userName))
}

// UpdateByID 根据 ID 更新用户数据(原 map 拼列名 → 类型化 Set,零值可写)
func (r *UserRepository) UpdateByID(ctx context.Context, userID uint64, req *dto.UpdateUserDTO) error {
	uw := r.m.NewUpdate()
	if req.Nickname != "" {
		uw.Set(func(u *entity.User) any { return &u.Nickname }, req.Nickname)
	}
	if req.Avatar != "" {
		uw.Set(func(u *entity.User) any { return &u.Avatar }, req.Avatar)
	}
	uw.Eq(func(u *entity.User) any { return &u.ID }, userID)
	_, err := r.m.Update(ctx, uw)
	return err
}
```

收益清单:软删条件消失(字段自动过滤)、列名编译期检查(写错字段编译不过)、方言转义(防保留字)、无 WHERE 更新被库主动拦截。

---

## 五、代码生成(类型化字段,推荐接入)

在项目里建生成入口,产出字段集包:

```
Memory-Record/
├── internal/
│   ├── gen/main.go        // 生成入口
│   └── query/fields_gen.go // 生成产物
```

`internal/gen/main.go`:

```go
//go:build ignore

package main

import (
	"log"

	"Memory-Record/internal/models/entity"
	"github.com/Lookfukc/gorm-max/gormmaxgen"
)

func main() {
	if err := gormmaxgen.Generate(gormmaxgen.Config{
		Out:    "internal/query/fields_gen.go", // 相对运行时工作目录(项目根)
		Models: []any{
			&entity.User{}, &entity.Book{}, &entity.Volume{},
			&entity.Chapter{}, &entity.Album{}, &entity.Photo{},
		},
		// Namer: 与运行时一致(默认 snake_case 时不用传)
	}); err != nil {
		log.Fatal(err)
	}
}
```

运行:

```bash
go run internal/gen/main.go     # 或在 main.go 挂 //go:generate 后执行 go generate ./...
```

使用(`internal/repositories` / `services` 里):

```go
import "Memory-Record/internal/query"

q := query.User
users, err := r.m.SelectList(ctx, r.m.NewQuery().
	Where(q.UserName.Eq(userName), q.Enabled.Eq(0)).
	Order(q.ID.Desc()))
```

类型映射规则(自动):

| 实体字段类型 | 生成字段类型 |
|---|---|
| `string` | `StringField`(带 Like 系方法) |
| `int/int8..64/uint 系/float` | 对应 `IntXField/UintXField/FloatXField` |
| `bool` / `time.Time` / `gorm.DeletedAt` / `[]byte` | `BoolField/TimeField/BytesField` |
| 自定义命名类型(`type Status int`、命名切片) | `TypedField[原类型]` **精确到原类型** |
| `map[string]any` 等 serializer JSON 列 | `GenericField`(值 any) |
| 自定义 Valuer 类型(`common.LocalDate`) | `TimeField`(按 GORM 语义,条件值传 `time.Time`) |

生成是确定性的(模型排序、字段按声明顺序、gofmt),重复生成零 diff。**模型变更后记得重新生成**——引用旧字段的地方直接编译报错,这正是 codegen 的核心收益。

---

## 六、Service 层与 Handler 使用

如果希望建立 service 级复用,在 `internal/services` 注入 gorm-max 的 `Service`(内嵌 Mapper 并提供 Query/SaveOrUpdate/Transaction 等):

```go
type UserService struct {
	*gorm_max.Service[entity.User]
	repo *repositories.UserRepository // 与现有 repo 共存,渐进替换
}

func NewUserService(db *gorm.DB) *UserService {
	return &UserService{Service: gorm_max.NewService[entity.User](db)}
}

// handler 里直接用
func (h *UserHandler) List(c *gin.Context) {
	page, err := h.userSvc.SelectPage(c.Request.Context(),
		h.userSvc.Query().Gt(func(u *entity.User) any { return &u.Enabled }, -1),
		&gorm_max.Page{Current: 1, Size: 20})
	// ...
}
```

---

## 七、事务改造

**Before**(`CreateWithTx` 手工传 tx):

```go
err := db.Transaction(func(tx *gorm.DB) error {
	return userRepo.CreateWithTx(ctx, tx, user)
})
```

**After**(事务糖,回调拿到绑好事务的对象):

```go
err := userSvc.Transaction(ctx, func(tx *gorm_max.Service[entity.User]) error {
	if err := tx.Insert(ctx, user); err != nil {
		return err
	}
	_, err := tx.Update(ctx, tx.NewUpdate().
		Set(func(u *entity.User) any { return &u.Nickname }, "tomcat").
		Eq(func(u *entity.User) any { return &u.ID }, user.ID))
	return err
})
```

error/panic 自动回滚,nil 自动提交,支持嵌套(SavePoint)。**回调内所有操作必须走 tx 参数**。

> Repository 若被绑定进事务:`userRepo.WithTx(tx)` —— 给 repository 加一个和 `Mapper.Tx` 同样的一行方法即可(克隆结构体、替换 db)。

---

## 八、渐进迁移策略

不必一次改完,推荐顺序:

1. **先引入插件与实体约定**(乐观锁/软删)——零业务代码改动,立即生效;
2. **新代码用 gorm-max,旧代码不动**——`Mapper.DB(ctx)` 逃生口保证复杂查询随时可用原生 gorm;
3. **按 repository 渐进替换**(每个方法签名不变,service/handler 零感知);
4. 最后接入 codegen,把高频查询切到类型化字段写法。

共存期间的判断依据:条件复杂、需要复用/分页/防呆的查询走 gorm-max;一次性复杂 join/窗口函数走 `Mapper.DB(ctx)` 原生 gorm——两者本来就是一体的。

---

## 九、注意事项清单

| # | 事项 |
|---|---|
| 1 | wrapper 实例非并发安全,每次请求新建(`Clone()` 后各自独立) |
| 2 | 嵌入结构体的**类型名必须导出**(`BaseTM` ✓,小写 `baseTM` 会被 gorm 整体跳过) |
| 3 | 字段选择器热路径用指针写法 `return &u.X`(O(1));值写法差分识别 O(字段数) |
| 4 | 带 `GroupBy`/`Distinct` 的 wrapper 不要用于 `SelectPage`(计数语义变化) |
| 5 | 乐观锁只对结构体更新生效;版本 0 视为未启用;插入时显式给初始版本 1 |
| 6 | `ConflictAction{UpdateAll: true}` 且唯一键非主键时,必须显式传 `Columns` |
| 7 | 无 WHERE 的 Update/Delete 默认拒绝(`gorm_max.WithAllowFullTable()` 显式放开) |
| 8 | 未找到记录统一返回 `gorm.ErrRecordNotFound`,service 层可 `errors.Is` 判断 |
| 9 | 自定义 Valuer 时间类型(LocalDate 等)在类型化字段里归一为 `TimeField`,条件值传 `time.Time` |
| 10 | 生成物不要手工编辑(`DO NOT EDIT`);模型变更后重新 `go generate` |
