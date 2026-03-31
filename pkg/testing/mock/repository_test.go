//go:build unit

package mock

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/sousair/gocore/pkg/database"
	"github.com/sousair/gocore/pkg/database/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestEntity struct {
	entity.BaseEntity
	Name   string `json:"name"`
	Status string `json:"status"`
	UserID uuid.UUID `json:"user_id"`
}

func (e TestEntity) GetID() uuid.UUID { return e.ID }

func TestCreate_AssignsID(t *testing.T) {
	repo := NewRepository[TestEntity]()
	ctx := context.Background()

	e, err := repo.Create(ctx, &TestEntity{Name: "test"})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, e.ID)
	assert.False(t, e.CreatedAt.IsZero())
	assert.Len(t, repo.Created, 1)
	assert.Len(t, repo.Records(), 1)
}

func TestFindOne_ByField(t *testing.T) {
	repo := NewRepository[TestEntity]()
	ctx := context.Background()

	repo.Create(ctx, &TestEntity{Name: "alice", Status: "active"})
	repo.Create(ctx, &TestEntity{Name: "bob", Status: "inactive"})

	found, err := repo.FindOne(ctx, &TestEntity{Name: "bob"})

	require.NoError(t, err)
	assert.Equal(t, "bob", found.Name)
}

func TestFindOne_ByID(t *testing.T) {
	repo := NewRepository[TestEntity]()
	ctx := context.Background()

	created, _ := repo.Create(ctx, &TestEntity{Name: "test"})
	found, err := repo.FindOne(ctx, &TestEntity{BaseEntity: entity.BaseEntity{ID: created.ID}})

	require.NoError(t, err)
	assert.Equal(t, created.ID, found.ID)
}

func TestFindOne_NotFound(t *testing.T) {
	repo := NewRepository[TestEntity]()
	ctx := context.Background()

	_, err := repo.FindOne(ctx, &TestEntity{Name: "nonexistent"})
	assert.ErrorIs(t, err, database.ErrNotFound)
}

func TestFindAll_ByQuery(t *testing.T) {
	repo := NewRepository[TestEntity]()
	ctx := context.Background()
	userID := uuid.New()

	repo.Create(ctx, &TestEntity{Name: "a", UserID: userID, Status: "active"})
	repo.Create(ctx, &TestEntity{Name: "b", UserID: userID, Status: "active"})
	repo.Create(ctx, &TestEntity{Name: "c", UserID: uuid.New(), Status: "active"})

	found, err := repo.FindAll(ctx, &TestEntity{UserID: userID})

	require.NoError(t, err)
	assert.Len(t, found, 2)
}

func TestFindAll_EmptyResult(t *testing.T) {
	repo := NewRepository[TestEntity]()
	ctx := context.Background()

	found, err := repo.FindAll(ctx, &TestEntity{Name: "nothing"})

	require.NoError(t, err)
	assert.Empty(t, found)
}

func TestUpdate(t *testing.T) {
	repo := NewRepository[TestEntity]()
	ctx := context.Background()

	created, _ := repo.Create(ctx, &TestEntity{Name: "original"})
	created.Name = "updated"
	updated, err := repo.Update(ctx, created)

	require.NoError(t, err)
	assert.Equal(t, "updated", updated.Name)
	assert.Len(t, repo.Updated, 1)
}

func TestDelete_SoftDelete(t *testing.T) {
	repo := NewRepository[TestEntity]()
	ctx := context.Background()

	created, _ := repo.Create(ctx, &TestEntity{Name: "to-delete"})
	err := repo.Delete(ctx, created)

	require.NoError(t, err)
	assert.Len(t, repo.Deleted, 1)
	assert.Len(t, repo.Records(), 0)    // not visible
	assert.Len(t, repo.AllRecords(), 1)  // still exists
}

func TestFindOne_SkipsSoftDeleted(t *testing.T) {
	repo := NewRepository[TestEntity]()
	ctx := context.Background()

	created, _ := repo.Create(ctx, &TestEntity{Name: "deleted"})
	repo.Delete(ctx, created)

	_, err := repo.FindOne(ctx, &TestEntity{Name: "deleted"})
	assert.ErrorIs(t, err, database.ErrNotFound)
}

func TestFindAll_SkipsSoftDeleted(t *testing.T) {
	repo := NewRepository[TestEntity]()
	ctx := context.Background()

	repo.Create(ctx, &TestEntity{Name: "active"})
	deleted, _ := repo.Create(ctx, &TestEntity{Name: "deleted"})
	repo.Delete(ctx, deleted)

	found, _ := repo.FindAll(ctx, &TestEntity{})
	assert.Len(t, found, 1)
	assert.Equal(t, "active", found[0].Name)
}

func TestTx_ExecutesFunction(t *testing.T) {
	repo := NewRepository[TestEntity]()
	ctx := context.Background()

	var called bool
	err := repo.Tx(ctx, func(ctx context.Context) error {
		called = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, called)
}

func TestReset(t *testing.T) {
	repo := NewRepository[TestEntity]()
	ctx := context.Background()

	repo.Create(ctx, &TestEntity{Name: "a"})
	repo.Create(ctx, &TestEntity{Name: "b"})

	repo.Reset()

	assert.Empty(t, repo.Records())
	assert.Empty(t, repo.Created)
	assert.Empty(t, repo.Updated)
	assert.Empty(t, repo.Deleted)
}

func TestCreateMany(t *testing.T) {
	repo := NewRepository[TestEntity]()
	ctx := context.Background()

	entities := []*TestEntity{
		{Name: "a"},
		{Name: "b"},
		{Name: "c"},
	}

	result, err := repo.CreateMany(ctx, entities)

	require.NoError(t, err)
	assert.Len(t, result, 3)
	assert.Len(t, repo.Records(), 3)
	assert.Len(t, repo.Created, 3)
}

func TestDeleteMany(t *testing.T) {
	repo := NewRepository[TestEntity]()
	ctx := context.Background()

	a, _ := repo.Create(ctx, &TestEntity{Name: "a"})
	b, _ := repo.Create(ctx, &TestEntity{Name: "b"})
	repo.Create(ctx, &TestEntity{Name: "c"})

	err := repo.DeleteMany(ctx, []*TestEntity{a, b})

	require.NoError(t, err)
	assert.Len(t, repo.Records(), 1)
	assert.Equal(t, "c", repo.Records()[0].Name)
}

func TestMatchesQuery_MultipleFields(t *testing.T) {
	repo := NewRepository[TestEntity]()
	ctx := context.Background()

	repo.Create(ctx, &TestEntity{Name: "task", Status: "pending", UserID: uuid.New()})
	target, _ := repo.Create(ctx, &TestEntity{Name: "task", Status: "done", UserID: uuid.New()})

	found, err := repo.FindOne(ctx, &TestEntity{Name: "task", Status: "done"})

	require.NoError(t, err)
	assert.Equal(t, target.ID, found.ID)
}
