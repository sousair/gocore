package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

type ctxKey struct{}

func FromContext(ctx context.Context) (*gorm.DB, error) {
	db, ok := ctx.Value(&ctxKey{}).(*gorm.DB)
	if !ok {
		return nil, errors.New("db not found in context")
	}

	return db, nil
}

func WithTx(ctx context.Context, db *gorm.DB) context.Context {
	return context.WithValue(ctx, &ctxKey{}, db)
}
