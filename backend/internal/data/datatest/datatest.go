// Package datatest is Trove's backend test harness: an ephemeral Postgres via
// testcontainers, migrated once per test binary, table-truncated between
// tests.
package datatest

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"trove/backend/internal/data"
)

var (
	once      sync.Once
	shared    *sql.DB
	sharedDSN string
	ctr       *postgres.PostgresContainer
	initErr   error
)

func init() {
	// Ryuk can't mount the socket on VM-backed Docker (Rancher Desktop etc.);
	// we own teardown via Main, so disable it. Honour an explicit override.
	if os.Getenv("TESTCONTAINERS_RYUK_DISABLED") == "" {
		_ = os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	}
	// testcontainers v0.42 doesn't read `docker context`; if DOCKER_HOST is
	// unset, resolve the active context's endpoint so Rancher Desktop / colima
	// users don't have to export it by hand.
	if os.Getenv("DOCKER_HOST") == "" {
		if h := activeDockerHost(); h != "" {
			_ = os.Setenv("DOCKER_HOST", h)
		}
	}
}

// activeDockerHost asks the docker CLI for the current context's endpoint.
// Best-effort: any failure returns "" and testcontainers falls back to its own
// detection.
func activeDockerHost() string {
	out, err := exec.Command("docker", "context", "inspect", "--format", "{{.Endpoints.docker.Host}}").Output()
	if err != nil {
		return ""
	}
	h := strings.TrimSpace(string(out))
	if strings.HasPrefix(h, "unix://") || strings.HasPrefix(h, "tcp://") {
		return h
	}
	return ""
}

// Main runs the package's tests and deterministically terminates the shared
// container afterwards. Wire it as the package's TestMain.
func Main(m *testing.M) int {
	code := m.Run()
	if ctr != nil {
		_ = ctr.Terminate(context.Background())
	}
	return code
}

// OpenTestDB returns a migrated DB backed by a single Postgres container
// shared across the test binary (started on first call). Each test gets a
// clean schema: every table is truncated when the test finishes (t.Cleanup).
//
// If Docker is unavailable the test is *skipped*, not failed — so
// `go test ./...` still runs on a Docker-less box. CI and a normal dev
// machine have Docker, so coverage is real where it counts.
func OpenTestDB(t *testing.T) *sql.DB {
	t.Helper()
	once.Do(func() { shared, ctr, initErr = startContainer() })
	if initErr != nil {
		if isDockerUnavailable(initErr) {
			t.Skipf("datatest: Docker unavailable — skipping DB-backed test: %v", initErr)
		}
		t.Fatalf("datatest: container init failed: %v", initErr)
	}
	t.Cleanup(func() { truncateAll(t, shared) })
	return shared
}

// DSN returns the shared container's connection string (URL form, sslmode
// disabled). For tests needing a raw connection outside the pool — e.g. a
// pq.Listener for LISTEN/NOTIFY. Ensures the container is up first (skips the
// test if Docker is unavailable, same as OpenTestDB).
func DSN(t *testing.T) string {
	t.Helper()
	OpenTestDB(t)
	return sharedDSN
}

func startContainer() (*sql.DB, *postgres.PostgresContainer, error) {
	ctx := context.Background()
	c, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("trove_test"),
		postgres.WithUsername("trove"),
		postgres.WithPassword("password"),
	)
	if err != nil {
		return nil, nil, err
	}
	dsn, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return nil, c, err
	}
	sharedDSN = dsn
	// data.InitDB connects (with its own ping-retry loop, ample for container
	// warm-up) and runs the embedded goose migrations — the exact prod path,
	// so tests exercise the real schema.
	db, err := data.InitDB(dsn)
	if err != nil {
		return nil, c, err
	}
	return db, c, nil
}

// truncateAll wipes every table (discovered dynamically, so new migrations
// need no harness change) and resets identities. RESTART IDENTITY CASCADE
// keeps per-test isolation without tx-rollback — which is off the table here
// because the code under test owns its own transactions.
func truncateAll(t *testing.T, db *sql.DB) {
	t.Helper()
	rows, err := db.Query(`
		SELECT tablename FROM pg_tables
		WHERE schemaname = 'public' AND tablename <> 'goose_db_version'
	`)
	if err != nil {
		t.Fatalf("datatest: list tables: %v", err)
	}
	var quoted []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			t.Fatalf("datatest: scan table name: %v", err)
		}
		quoted = append(quoted, pq.QuoteIdentifier(name))
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("datatest: iterate tables: %v", err)
	}
	if len(quoted) == 0 {
		return
	}
	if _, err := db.Exec(fmt.Sprintf(
		"TRUNCATE %s RESTART IDENTITY CASCADE", strings.Join(quoted, ", "),
	)); err != nil {
		t.Fatalf("datatest: truncate: %v", err)
	}
}

func isDockerUnavailable(err error) bool {
	s := strings.ToLower(err.Error())
	for _, marker := range []string{
		"cannot connect to the docker daemon",
		"docker daemon",
		"rootless docker not found",
		"failed to find docker",
		"docker host is not set",
		"error during connect",
	} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}

// --- Seed helpers (the reusable fixture surface) ---

// seedTx runs fn in a WithRetry transaction (the only write path — there is no
// standalone mutation mode), failing the test on error.
func seedTx(t *testing.T, db *sql.DB, fn func(tx *sql.Tx) error) {
	t.Helper()
	if err := data.WithRetry(context.Background(), db, fn); err != nil {
		t.Fatalf("datatest: seed tx: %v", err)
	}
}

// SeedUser inserts a user and returns its ID. Call before SeedProject — the
// owner_id FK requires the row.
func SeedUser(t *testing.T, db *sql.DB) string {
	t.Helper()
	id := uuid.NewString()
	if err := data.UpsertUser(context.Background(), db, id, id+"@test.local"); err != nil {
		t.Fatalf("datatest: seed user: %v", err)
	}
	return id
}

// SeedProject creates a project owned by ownerID (unique slug).
func SeedProject(t *testing.T, db *sql.DB, ownerID string) *data.Project {
	t.Helper()
	slug := "p-" + uuid.NewString()[:8]
	var p *data.Project
	seedTx(t, db, func(tx *sql.Tx) error {
		var err error
		p, err = data.CreateProject(context.Background(), tx, ownerID, slug, "Test "+slug, nil, nil, nil)
		return err
	})
	return p
}

// SeedItem creates a task item in the given project.
func SeedItem(t *testing.T, db *sql.DB, projectID, creatorID string) *data.Item {
	t.Helper()
	var it *data.Item
	seedTx(t, db, func(tx *sql.Tx) error {
		var err error
		it, err = data.CreateItem(context.Background(), tx, projectID, creatorID, data.ItemKindTask, "Test item", nil)
		return err
	})
	return it
}
