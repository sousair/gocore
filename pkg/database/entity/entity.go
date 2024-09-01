package entity

import (
	"time"

	"gorm.io/gorm"
)

type (
	Entity interface {
		GetID() uint
	}

	BaseEntity struct {
		ID        uint           `json:"id" param:"id" gorm:"primary_key"`
		CreatedAt time.Time      `json:"created_at" gorm:"column:created_at"`
		UpdatedAt time.Time      `json:"updated_at" gorm:"column:updated_at"`
		DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"column:deleted_at"`
	}
)

var _ Entity = (*BaseEntity)(nil)

func (e BaseEntity) GetID() uint {
	return e.ID
}
