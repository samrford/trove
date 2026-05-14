package data

import (
	"database/sql"
	"embed"
	"fmt"
	"log"
	"time"

	"github.com/lib/pq"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

// Postgres SQLSTATE codes & schema constraint names we match on to classify
// errors. Exported so handler-layer tests can construct identical pq.Error
// shapes without re-stringifying values by hand.
const (
	PGUniqueViolation pq.ErrorCode = "23505"

	ConstraintTagsOwnerSlugUnique       = "tags_owner_slug_unique"
	ConstraintTagsOwnerNormalisedUnique = "tags_owner_normalised_unique"
)

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
