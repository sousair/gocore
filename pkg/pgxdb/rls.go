package pgxdb

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultGUC = "app.current_user_id"

// RLS runs queries inside transactions scoped to a user via a session GUC,
// for the fail-closed row-level-security pattern. Construct via NewRLS; the
// zero value is not usable.
type RLS struct {
	pool *pgxpool.Pool
	guc  string
}

// RLSOption configures an RLS instance.
type RLSOption func(*RLS)

// WithGUC overrides the session variable name used to convey the current user
// identity to RLS policies. The default is "app.current_user_id".
func WithGUC(name string) RLSOption {
	return func(r *RLS) {
		r.guc = name
	}
}

// NewRLS constructs an RLS using the given pool and options.
func NewRLS(pool *pgxpool.Pool, opts ...RLSOption) *RLS {
	r := &RLS{pool: pool, guc: defaultGUC}
	for _, o := range opts {
		o(r)
	}
	return r
}

// InUserTx runs fn inside a transaction with the GUC set transaction-locally
// to userID (via set_config, since SET LOCAL takes no bind parameters), so
// row-level-security policies see the current user. Any error rolls back.
func (r *RLS) InUserTx(ctx context.Context, userID uuid.UUID, fn func(pgx.Tx) error) error {
	if r.pool == nil {
		return errors.New("pgxdb: nil pool — construct RLS via NewRLS with a live pool")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("pgxdb: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx,
		"SELECT set_config($1, $2, true)",
		r.guc, userID.String(),
	); err != nil {
		return fmt.Errorf("pgxdb: set user context: %w", err)
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
