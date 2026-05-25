package data

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/lib/pq"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

// Mutation functions take *sql.Tx: the only way to get a
// *sql.Tx is from WithRetry, so the compiler enforces that every write runs in
// a retryable transaction (paired atomically with its activity row).

// InitDB connects to the database, retries on failure, then runs migrations.
func InitDB(connURL string) (*sql.DB, error) {
	db, err := sql.Open("postgres", connURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	var pingErr error
	for i := 0; i < 15; i++ {
		pingErr = db.Ping()
		if pingErr == nil {
			break
		}
		log.Printf("Waiting for database to be ready... attempt %d/15", i+1)
		time.Sleep(2 * time.Second)
	}
	if pingErr != nil {
		return nil, fmt.Errorf("failed to ping database after retries: %w", pingErr)
	}

	log.Println("Successfully connected to the PostgreSQL database")

	// Explicit pool bounds. SSE handlers can hold a pool conn during reconnect
	// catch-up; unbounded defaults would let many concurrent streams exhaust
	// Postgres. (The pq.Listener uses its own dedicated conn outside this pool.)
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxIdleTime(5 * time.Minute)
	db.SetConnMaxLifetime(30 * time.Minute)

	goose.SetBaseFS(embedMigrations)

	if err := goose.SetDialect("postgres"); err != nil {
		return nil, fmt.Errorf("failed to set goose dialect: %w", err)
	}

	if err := goose.Up(db, "migrations"); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	log.Println("Database migrations applied successfully")

	return db, nil
}

const maxTxAttempts = 3

// WithRetry runs fn inside a transaction and commits it as one unit. The whole
// unit is retried (bounded, jittered) only on transient conflicts
// (serialization_failure / deadlock_detected), so fn may be invoked more than
// once: it must be a pure function of database state with no external side
// effects.
func WithRetry(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error {
	var err error
	for attempt := 0; attempt < maxTxAttempts; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		err = runTxAttempt(ctx, db, fn)
		if err == nil {
			return nil
		}
		if !isRetryableTxErr(err) || ctx.Err() != nil {
			return err
		}
		if attempt == maxTxAttempts-1 {
			break // out of attempts — don't sleep before giving up
		}

		// Jittered backoff: ~5ms, ~10ms growing, +0–50%.
		base := time.Duration(5<<attempt) * time.Millisecond
		sleep := base + time.Duration(rand.Int63n(int64(base)/2+1))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sleep):
		}
	}
	return err
}

// runTxAttempt runs fn in one transaction and commits it. A panic in fn rolls
// the tx back before unwinding so a buggy caller can't leak the connection.
func runTxAttempt(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback() // no-op if the failed commit already finalised the tx
		return err
	}
	return nil
}

// isRetryableTxErr reports whether err is a transient Postgres conflict worth
// retrying the whole transaction for. Traverses wrapping — data fns %w-wrap.
func isRetryableTxErr(err error) bool {
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return false
	}
	switch pqErr.Code {
	case "40001", "40P01": // serialization_failure, deadlock_detected
		return true
	}
	return false
}
