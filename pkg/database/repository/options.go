package repository

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Option func(*gorm.DB) *gorm.DB

func WithPreload(association string) Option {
	return func(db *gorm.DB) *gorm.DB {
		return db.Preload(association)
	}
}

func WithPreloadAll() Option {
	return func(db *gorm.DB) *gorm.DB {
		return db.Preload(clause.Associations)
	}
}
