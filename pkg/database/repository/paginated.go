package repository

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"

	"github.com/sousair/gocore/pkg/database"
	"github.com/sousair/gocore/pkg/database/entity"
	"gorm.io/gorm"
)

type (
	OrderBy struct {
		Field     string `json:"field"`
		Direction string `json:"direction"` // ASC or DESC
	}
	BasePaginationQuery[Query any] struct {
		Query   *Query   `json:"query"`
		PerPage int      `json:"per_page"`
		Page    int      `json:"page"`
		OrderBy *OrderBy `json:"order_by"`
	}

	PaginationResponse[E entity.Entity] struct {
		Items      []*E  `json:"items"`
		Total      int64 `json:"total"`
		Page       int   `json:"page"`
		TotalPages int   `json:"total_pages"`
	}

	PaginationIterator[E entity.Entity] func() (*PaginationResponse[E], error)
)

func (pr *PaginationResponse[Entity]) HasNextPage() bool {
	return pr.Page < pr.TotalPages
}

type PaginatedRepository[E entity.Entity, query any] interface {
	FindAllWithPages(context.Context, *BasePaginationQuery[query], ...Option) (*PaginationResponse[E], error)
	FindAllWithPagesIter(context.Context, *BasePaginationQuery[query], ...Option) (PaginationIterator[E], error)
}

type paginateRepository[E entity.Entity, query any] struct {
	db *gorm.DB
}

var _ PaginatedRepository[entity.Entity, any] = (*paginateRepository[entity.Entity, any])(nil)

func NewPaginated[E entity.Entity, query any](db *gorm.DB) (*paginateRepository[E, query], error) {
	var rawEntity any = new(E)
	entity, ok := rawEntity.(entity.Entity)
	if !ok {
		return nil, database.ErrBadEntity
	}

	if os.Getenv("DB_AUTO_MIGRATE") == "true" {
		if err := db.AutoMigrate(entity); err != nil {
			return nil, err
		}
	}

	return &paginateRepository[E, query]{db}, nil
}

func (r paginateRepository[E, query]) FindAllWithPages(
	ctx context.Context,
	baseQuery *BasePaginationQuery[query],
	opts ...Option,
) (*PaginationResponse[E], error) {
	tx := r.db
	if dbTx, err := FromContext(ctx); err == nil {
		tx = dbTx
	}

	for _, opt := range opts {
		tx = opt(tx)
	}

	tx = tx.Model(new(E))

	res := &PaginationResponse[E]{}
	if err := tx.
		Count(&res.Total).Error; err != nil {
		return nil, err
	}

	if baseQuery.OrderBy != nil {
		if baseQuery.OrderBy.Field != "" && baseQuery.OrderBy.Direction != "" {
			tx = tx.Order(fmt.Sprintf("%s %s",
				baseQuery.OrderBy.Field,
				baseQuery.OrderBy.Direction,
			))
		}
	}

	page := 1
	if baseQuery.PerPage != 0 && baseQuery.Page != 0 {
		offset := (baseQuery.Page - 1) * baseQuery.PerPage
		pages := math.Ceil(float64(res.Total) / float64(baseQuery.PerPage))
		if offset >= int(res.Total) {
			pages = 1
		}

		res.TotalPages = int(pages)
		page = baseQuery.Page
		tx = tx.Limit(baseQuery.PerPage).Offset(offset)
	}

	var data []*E
	if err := tx.Find(&data).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, database.ErrNotFound
		}

		return nil, err
	}

	res.Page = page
	res.Items = data

	return res, nil
}

// Remember to call the PaginationResponse.HasNextPage() to check if there are more pages
func (r paginateRepository[E, query]) FindAllWithPagesIter(
	ctx context.Context,
	baseQuery *BasePaginationQuery[query],
	opts ...Option,
) (PaginationIterator[E], error) {
	tx := r.db
	if dbTx, err := FromContext(ctx); err == nil {
		tx = dbTx
	}

	for _, opt := range opts {
		tx = opt(tx)
	}

	tx = tx.Model(new(E))

	res := &PaginationResponse[E]{}
	if err := tx.
		Count(&res.Total).Error; err != nil {
		return nil, err
	}

	if baseQuery.OrderBy != nil {
		if baseQuery.OrderBy.Field != "" && baseQuery.OrderBy.Direction != "" {
			tx = tx.Order(fmt.Sprintf("%s %s",
				baseQuery.OrderBy.Field,
				baseQuery.OrderBy.Direction,
			))
		}
	}

	return func() (*PaginationResponse[E], error) {
		page := 1
		if baseQuery.PerPage != 0 && baseQuery.Page != 0 {
			offset := (baseQuery.Page - 1) * baseQuery.PerPage
			pages := math.Ceil(float64(res.Total) / float64(baseQuery.PerPage))
			if offset >= int(res.Total) {
				pages = 1
			}

			res.TotalPages = int(pages)
			page = baseQuery.Page
			tx = tx.Limit(baseQuery.PerPage).Offset(offset)
		}

		var data []*E
		if err := tx.Find(&data).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, database.ErrNotFound
			}

			return nil, err
		}

		res.Page = page
		res.Items = data

		baseQuery.Page++
		return res, nil
	}, nil
}
