//go:build unit

package mock

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sousair/gocore/pkg/database"
	"github.com/sousair/gocore/pkg/database/entity"
	"github.com/sousair/gocore/pkg/database/repository"
	"gorm.io/gorm"
)

type Repository[T entity.Entity] struct {
	mu      sync.RWMutex
	records []*T

	Created []*T
	Updated []*T
	Deleted []*T
}

var _ repository.Repository[entity.BaseEntity] = (*Repository[entity.BaseEntity])(nil)

func NewRepository[T entity.Entity]() *Repository[T] {
	return &Repository[T]{}
}

func (r *Repository[T]) DB() *gorm.DB {
	return nil
}

func (r *Repository[T]) Tx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func (r *Repository[T]) Create(ctx context.Context, e *T, opts ...repository.Option) (*T, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	val := reflect.ValueOf(e).Elem()
	base := findBaseEntity(val)
	if base != nil {
		if base.ID == uuid.Nil {
			base.ID, _ = uuid.NewV7()
		}
		base.CreatedAt = time.Now()
		base.UpdatedAt = time.Now()
	}

	r.records = append(r.records, e)
	r.Created = append(r.Created, e)
	return e, nil
}

func (r *Repository[T]) CreateMany(ctx context.Context, entities []*T, opts ...repository.Option) ([]*T, error) {
	for _, e := range entities {
		if _, err := r.Create(ctx, e, opts...); err != nil {
			return nil, err
		}
	}
	return entities, nil
}

func (r *Repository[T]) FindOne(ctx context.Context, query *T, opts ...repository.Option) (*T, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	queryVal := reflect.ValueOf(query).Elem()

	for _, record := range r.records {
		if isDeleted(reflect.ValueOf(record).Elem()) {
			continue
		}
		if matchesQuery(reflect.ValueOf(record).Elem(), queryVal) {
			return record, nil
		}
	}

	return nil, database.ErrNotFound
}

func (r *Repository[T]) FindAll(ctx context.Context, query *T, opts ...repository.Option) ([]*T, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	queryVal := reflect.ValueOf(query).Elem()
	var result []*T

	for _, record := range r.records {
		if isDeleted(reflect.ValueOf(record).Elem()) {
			continue
		}
		if matchesQuery(reflect.ValueOf(record).Elem(), queryVal) {
			result = append(result, record)
		}
	}

	return result, nil
}

func (r *Repository[T]) Update(ctx context.Context, e *T, opts ...repository.Option) (*T, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	val := reflect.ValueOf(e).Elem()
	base := findBaseEntity(val)
	if base != nil {
		base.UpdatedAt = time.Now()
	}

	r.Updated = append(r.Updated, e)
	return e, nil
}

func (r *Repository[T]) UpdateMany(ctx context.Context, where *T, data *T, opts ...repository.Option) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	whereVal := reflect.ValueOf(where).Elem()
	dataVal := reflect.ValueOf(data).Elem()

	for _, record := range r.records {
		if matchesQuery(reflect.ValueOf(record).Elem(), whereVal) {
			copyNonZeroFields(reflect.ValueOf(record).Elem(), dataVal)
			r.Updated = append(r.Updated, record)
		}
	}

	return nil
}

func (r *Repository[T]) Delete(ctx context.Context, e *T) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.Deleted = append(r.Deleted, e)

	val := reflect.ValueOf(e).Elem()
	base := findBaseEntity(val)
	if base != nil {
		base.DeletedAt = gorm.DeletedAt{Time: time.Now(), Valid: true}
	}

	return nil
}

func (r *Repository[T]) DeleteMany(ctx context.Context, entities []*T) error {
	for _, e := range entities {
		if err := r.Delete(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository[T]) Reload(ctx context.Context, e *T, opts ...repository.Option) error {
	found, err := r.FindOne(ctx, e, opts...)
	if err != nil {
		return err
	}

	reflect.ValueOf(e).Elem().Set(reflect.ValueOf(found).Elem())
	return nil
}

func (r *Repository[T]) Records() []*T {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*T
	for _, record := range r.records {
		if !isDeleted(reflect.ValueOf(record).Elem()) {
			result = append(result, record)
		}
	}
	return result
}

func (r *Repository[T]) AllRecords() []*T {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*T, len(r.records))
	copy(result, r.records)
	return result
}

func (r *Repository[T]) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.records = nil
	r.Created = nil
	r.Updated = nil
	r.Deleted = nil
}

func findBaseEntity(v reflect.Value) *entity.BaseEntity {
	if v.Kind() != reflect.Struct {
		return nil
	}

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		if field.Type() == reflect.TypeOf(entity.BaseEntity{}) && field.CanAddr() {
			return field.Addr().Interface().(*entity.BaseEntity)
		}
	}
	return nil
}

func matchesQuery(record, query reflect.Value) bool {
	if record.Kind() != reflect.Struct || query.Kind() != reflect.Struct {
		return false
	}

	for i := 0; i < query.NumField(); i++ {
		queryField := query.Field(i)
		fieldType := query.Type().Field(i)

		if fieldType.Anonymous && queryField.Kind() == reflect.Struct {
			recordEmbedded := record.FieldByName(fieldType.Name)
			if recordEmbedded.IsValid() {
				if !matchesQuery(recordEmbedded, queryField) {
					return false
				}
			}
			continue
		}

		if queryField.IsZero() {
			continue
		}

		fieldName := fieldType.Name
		if queryField.Kind() == reflect.Ptr && queryField.Type().Elem().Kind() == reflect.Struct {
			if !strings.HasSuffix(fieldName, "ID") {
				continue
			}
		}
		if queryField.Kind() == reflect.Slice {
			continue
		}

		recordField := record.FieldByName(fieldName)
		if !recordField.IsValid() {
			continue
		}

		if !reflect.DeepEqual(queryField.Interface(), recordField.Interface()) {
			return false
		}
	}

	return true
}

func isDeleted(v reflect.Value) bool {
	base := findBaseEntity(v)
	if base == nil {
		return false
	}
	return base.DeletedAt.Valid
}

func copyNonZeroFields(dst, src reflect.Value) {
	for i := 0; i < src.NumField(); i++ {
		srcField := src.Field(i)
		if srcField.IsZero() {
			continue
		}
		dstField := dst.FieldByName(src.Type().Field(i).Name)
		if dstField.IsValid() && dstField.CanSet() {
			dstField.Set(srcField)
		}
	}
}
