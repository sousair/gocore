package repository

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Option func(*gorm.DB) *gorm.DB

func WithDeleted() Option {
	return func(db *gorm.DB) *gorm.DB {
		return db.Unscoped()
	}
}

func WithRelation(relations ...string) Option {
	return func(db *gorm.DB) *gorm.DB {
		for _, relation := range relations {
			db = db.Preload(relation)
		}

		return db
	}
}

func WithAllRelations() Option {
	return func(db *gorm.DB) *gorm.DB {
		return db.Preload(clause.Associations)
	}
}

func WithRowLock(allowRead bool) Option {
	return func(db *gorm.DB) *gorm.DB {
		lock := clause.Locking{Strength: clause.LockingStrengthUpdate}
		if allowRead {
			lock = clause.Locking{Strength: clause.LockingStrengthShare}
		}

		return db.Clauses(lock)
	}
}
