package db

import (
	"context"
	"database/sql"
	"fmt"
)

// InTx runs fn in a database transaction. Queries created from an existing
// transaction cannot start a nested transaction.
func (q *Queries) InTx(ctx context.Context, fn func(*Queries) error) error {
	database, ok := q.db.(*sql.DB)
	if !ok {
		return fmt.Errorf("db: cannot start transaction from %T", q.db)
	}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(q.WithTx(tx)); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
