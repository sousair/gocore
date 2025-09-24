package repository

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Option func(*gorm.DB) *gorm.DB

func WithRelation(association string) Option {
	return func(d *gorm.DB) *gorm.DB {
		return d.Preload(association)
	}
}

func WithAllRelations() Option {
	return func(db *gorm.DB) *gorm.DB {
		return db.Preload(clause.Associations)
	}
}
