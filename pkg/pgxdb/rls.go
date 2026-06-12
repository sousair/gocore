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
// implementing the fail-closed row-level-security pattern. See the package
// documentation for the required policy shape. Construct via NewRLS; the zero
// value is not usable.
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

// InUserTx begins a transaction, sets the GUC transaction-locally via a
// parameterized set_config call (SET LOCAL cannot accept bind parameters),
// invokes fn, and commits. Any error — including one returned by fn — causes
// a rollback. The GUC is set with the third argument to set_config as true so
// it is scoped to the transaction and reverts automatically on commit or
// rollback; see the package documentation for the NULLIF policy gotcha.
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
