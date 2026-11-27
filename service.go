package gorm_max

import (
	"context"

	"gorm.io/gorm"
)

// Service 对应 MP 的 IService:内嵌 Mapper,追加业务常用组合方法。
type Service[T any] struct {
	*Mapper[T]
}

func NewService[T any](db *gorm.DB, opts ...MapperOption) *Service[T] {
	return &Service[T]{Mapper: NewMapper[T](db, opts...)}
}

// Query 等价 MP 的 lambdaQuery(),链式构造后传给 SelectList/SelectPage 等。
func (s *Service[T]) Query() *QueryWrapper[T] { return s.NewQuery() }

// Transaction 事务糖(同 Mapper.Transaction,回调拿到的是绑定事务的 Service):
//
//	err := svc.Transaction(ctx, func(tx *Service[User]) error {
//		return tx.Insert(ctx, u)
//	})
//
// 回调返回 error 自动回滚、nil 自动提交(panic 同样回滚后继续上抛),支持嵌套(内部 SavePoint)。
// 注意:回调内所有操作必须走 tx 参数,混用外层 svc 的语句会脱离事务。
func (s *Service[T]) Transaction(ctx context.Context, fn func(tx *Service[T]) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&Service[T]{Mapper: s.Tx(tx)})
	})
}

// Exists 由内嵌 Mapper 提升,无需在 Service 重复实现。

// SaveOrUpdate 主键为空则插入,否则按主键更新(不检查存在性)。
func (s *Service[T]) SaveOrUpdate(ctx context.Context, entity *T) error {
	if _, ok := s.res.pkValue(entity); !ok {
		return s.Insert(ctx, entity)
	}
	_, err := s.UpdateByID(ctx, entity)
	return err
}

// SaveOrUpdateBatch 批量版:无主键的聚合走批量插入,有主键的逐条更新。
func (s *Service[T]) SaveOrUpdateBatch(ctx context.Context, entities []*T, batchSize ...int) error {
	var inserts []*T
	for _, e := range entities {
		if _, ok := s.res.pkValue(e); !ok {
			inserts = append(inserts, e)
			continue
		}
		if _, err := s.UpdateByID(ctx, e); err != nil {
			return err
		}
	}
	if len(inserts) > 0 {
		return s.InsertBatch(ctx, inserts, batchSize...)
	}
	return nil
}
