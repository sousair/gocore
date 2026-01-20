package entity

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type (
	Entity interface {
		GetID() uuid.UUID
	}

	BaseEntity struct {
		ID        uuid.UUID      `json:"id" param:"id" gorm:"primary_key"`
		CreatedAt time.Time      `json:"created_at" gorm:"column:created_at"`
		UpdatedAt time.Time      `json:"updated_at" gorm:"column:updated_at"`
		DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"column:deleted_at"`
	}
)

var _ Entity = (*BaseEntity)(nil)

func (e BaseEntity) GetID() uuid.UUID {
	return e.ID
}

func (e *BaseEntity) BeforeCreate(tx *gorm.DB) (err error) {
	if e.ID == uuid.Nil {
		e.ID, err = uuid.NewV7()
		if err != nil {
			return err
		}
	}

	return nil
}
