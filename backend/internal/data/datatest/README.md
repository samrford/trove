# datatest

Trove's backend test harness: an ephemeral Postgres (via testcontainers),
migrated with the real `migrations/`, table-truncated between tests.

**You only need 4 functions. Everything else in this package is one-time Docker
plumbing you never touch.**

## Using it (copy this)

```go
package yourpkg_test // external test package — required (datatest imports data)

import (
	"os"
	"testing"

	"trove/backend/internal/data/datatest"
)

// Required once per test package — wires container teardown.
func TestMain(m *testing.M) { os.Exit(datatest.Main(m)) }

func TestSomething(t *testing.T) {
	db := datatest.OpenTestDB(t)            // migrated DB, clean schema per test
	user := datatest.SeedUser(t, db)        // → userID
	project := datatest.SeedProject(t, db, user)
	item := datatest.SeedItem(t, db, project.ID, user)

	// ...assert against real Postgres...
}
```

That's the whole API: `Main`, `OpenTestDB`, `SeedUser`, `SeedProject`,
`SeedItem`.

## Three rules

1. **`func TestMain(m *testing.M) { os.Exit(datatest.Main(m)) }`** — one line,
   once per test package. Without it the container isn't cleaned up.
2. **External test package** (`yourpkg_test`, not `yourpkg`) — datatest imports
   `data`, so an internal test package would be an import cycle.
3. **No `t.Parallel()`** while sharing the DB — tests share one container and
   truncate between runs; parallel would race. Serial is the intended model.

Each test gets a clean schema automatically (every table truncated on
`t.Cleanup`). Mutations take `*sql.Tx`, so to call one in a test, go through
`data.WithRetry` (or use the `Seed*` helpers, which already do).

## A few notes

- **Ryuk disabled in-code** — testcontainers' reaper bind-mounts the Docker
  socket, which fails on VM-backed Docker (Rancher Desktop, colima). We tear
  the container down deterministically in `Main` instead.
- **`DOCKER_HOST` auto-detected** from the active `docker context` — so
  Rancher Desktop / colima / Docker Desktop all work with zero config.
- **Singleton container + `TRUNCATE … RESTART IDENTITY CASCADE`** — one
  container per `go test` run (fast), per-test isolation without tx-rollback
  (the code under test owns its own transactions, so rollback isolation would
  mask it).


## No Docker?

Tests **skip**, they don't fail (`"datatest: Docker unavailable — skipping"`).
CI and a normal dev box have Docker, so coverage is real where it counts.
