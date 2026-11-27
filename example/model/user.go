// Package model 示例实体。真实项目里模型通常独立成包,便于 codegen 引用。
package model

import (
	"time"

	"gorm.io/gorm"
)

// User 示例模型。
type User struct {
	ID        int64  `gorm:"primaryKey"`
	Name      string `gorm:"size:64;index"`
	Age       int
	Email     string
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"` // 逻辑删除,等价 MP 的 @TableLogic
}
