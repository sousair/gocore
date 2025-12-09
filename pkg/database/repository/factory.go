package repository

import (
	"context"
	"database/sql"
	"errors"
	"os"

	"github.com/sousair/gocore/pkg/database"
	"github.com/sousair/gocore/pkg/database/entity"
	"gorm.io/gorm"
)

type Repository[T entity.Entity] interface {
	DB() *gorm.DB
	Tx(context.Context, func(context.Context) error) error

	FindOne(context.Context, *T, ...Option) (*T, error)
	FindAll(context.Context, *T, ...Option) ([]*T, error)

	Create(context.Context, *T, ...Option) (*T, error)
	CreateMany(context.Context, []*T, ...Option) ([]*T, error)

	Update(context.Context, *T, ...Option) (*T, error)
	UpdateMany(ctx context.Context, where *T, data *T, opts ...Option) error

	Delete(context.Context, *T) error
	DeleteMany(context.Context, []*T) error

	Reload(context.Context, *T, ...Option) error
}

type repository[T entity.Entity] struct {
	db *gorm.DB
}

var _ Repository[entity.Entity] = (*repository[entity.Entity])(nil)

func New[T entity.Entity](db *gorm.DB) (*repository[T], error) {
	var rawEntity any = new(T)

	entity, ok := rawEntity.(entity.Entity)

	if !ok {
		return nil, database.ErrBadEntity
	}

	if os.Getenv("DB_AUTO_MIGRATE") == "true" {
		if err := db.AutoMigrate(entity); err != nil {
			return nil, err
		}
	}

	return &repository[T]{db}, nil
}

func (r *repository[T]) DB() *gorm.DB {
	return r.db
}

func (r *repository[T]) Tx(ctx context.Context, txFn func(context.Context) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return txFn(WithTx(ctx, tx))
	})
}

func (r *repository[T]) FindOne(ctx context.Context, entity *T, opts ...Option) (*T, error) {
	tx := r.db
	if dbTx, err := FromContext(ctx); err == nil {
		tx = dbTx
	}

	for _, opt := range opts {
		tx = opt(tx)
	}

	if err := tx.Model(entity).Where(entity).Take(entity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, database.ErrNotFound
		}
		return nil, err
	}

	return entity, nil
}

func (r *repository[T]) FindAll(ctx context.Context, query *T, opts ...Option) ([]*T, error) {
	tx := r.db
	if dbTx, err := FromContext(ctx); err == nil {
		tx = dbTx
	}

	for _, opt := range opts {
		tx = opt(tx)
	}

	var res []*T
	if err := tx.Where(query).Find(&res).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, database.ErrNotFound
		}
		return nil, err
	}

	return res, nil
}

func (r *repository[T]) Create(ctx context.Context, entity *T, opts ...Option) (*T, error) {
	tx := r.db
	if dbTx, err := FromContext(ctx); err == nil {
		tx = dbTx
	}

	for _, opt := range opts {
		tx = opt(tx)
	}

	if err := tx.Create(entity).Error; err != nil {
		return nil, err
	}

	return entity, nil
}

func (r *repository[T]) CreateMany(ctx context.Context, entities []*T, opts ...Option) ([]*T, error) {
	tx := r.db
	if dbTx, err := FromContext(ctx); err == nil {
		tx = dbTx
	}

	for _, opt := range opts {
		tx = opt(tx)
	}

	if err := tx.Create(entities).Error; err != nil {
		return nil, err
	}

	return entities, nil
}

func (r *repository[T]) Update(ctx context.Context, entity *T, opts ...Option) (*T, error) {
	tx := r.db
	if dbTx, err := FromContext(ctx); err == nil {
		tx = dbTx
	}

	for _, opt := range opts {
		tx = opt(tx)
	}

	if err := tx.Model(entity).Updates(entity).Error; err != nil {
		return nil, err
	}

	return entity, nil
}

func (r *repository[T]) UpdateMany(ctx context.Context, where *T, data *T, opts ...Option) error {
	tx := r.db
	if dbTx, err := FromContext(ctx); err == nil {
		tx = dbTx
	}

	for _, opt := range opts {
		tx = opt(tx)
	}

	return tx.Model(where).Where(where).Updates(data).Error
}

func (r *repository[T]) Delete(ctx context.Context, entity *T) error {
	tx := r.db
	if dbTx, err := FromContext(ctx); err == nil {
		tx = dbTx
	}

	if err := tx.Delete(entity).Error; err != nil {
		return err
	}

	return nil
}

func (r *repository[T]) DeleteMany(ctx context.Context, entities []*T) error {
	tx := r.db
	if dbTx, err := FromContext(ctx); err == nil {
		tx = dbTx
	}

	if err := tx.Delete(entities).Error; err != nil {
		return err
	}

	return nil
}

func (r *repository[T]) Reload(ctx context.Context, entity *T, opts ...Option) error {
	tx := r.db
	if dbTx, err := FromContext(ctx); err == nil {
		tx = dbTx
	}

	for _, opt := range opts {
		tx = opt(tx)
	}

	if err := tx.First(entity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return database.ErrNotFound
		}
		return err
	}

	return nil
}

func (r *repository[T]) Query(ctx context.Context, query string, values ...any) (*sql.Rows, error) {
	q := r.db.Raw(query, values)
	rows, err := q.Rows()
	defer rows.Close()

	if err != nil {
		return nil, err
	}

	return rows, nil
}
