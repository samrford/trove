package data

import (
	"context"
	"database/sql"
)

// UpsertUser ensures a row exists for the given Supabase user ID, updating
// email if it has changed. Idempotent — safe to call on every authenticated
// request.
func UpsertUser(ctx context.Context, db *sql.DB, id, email string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO users (id, email)
		VALUES ($1, $2)
		ON CONFLICT (id) DO UPDATE SET email = EXCLUDED.email
	`, id, email)
	return err
}
