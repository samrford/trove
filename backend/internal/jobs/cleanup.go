// Jobs for background cleanup tasks.
package jobs

import (
	"context"
	"database/sql"
	"log"
	"time"

	"trove/backend/internal/data/storage"
)

// orphanLockID is the constant passed to pg_try_advisory_lock so multi-
// instance deploys never run the sweep concurrently. Arbitrary 64-bit value;
// the only requirement is that nothing else in the codebase reuses it.
const orphanLockID int64 = 0x74726f7665617474 // "troveatt"

// OrphanGracePeriod is how long an unreferenced storage object must sit
// untouched before the sweep purges it.
const OrphanGracePeriod = 1 * time.Hour

// OrphanSweepInterval is how often the daemon checks for orphans.
const OrphanSweepInterval = 24 * time.Hour

// RunOrphanSweep blocks until ctx is cancelled, sweeping orphaned storage
// objects on a daily ticker. Safe to launch as `go RunOrphanSweep(ctx, ...)`.
// First sweep fires at startup so we don't have to wait a day to find out it
// works.
func RunOrphanSweep(ctx context.Context, db *sql.DB, store storage.FileStore) {
	if store == nil {
		log.Println("orphan sweep: storage not configured; skipping")
		return
	}

	if err := sweepOnce(ctx, db, store); err != nil {
		log.Printf("orphan sweep (initial): %v", err)
	}

	ticker := time.NewTicker(OrphanSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("orphan sweep: shutting down")
			return
		case <-ticker.C:
			if err := sweepOnce(ctx, db, store); err != nil {
				log.Printf("orphan sweep: %v", err)
			}
		}
	}
}

// sweepOnce performs a single sweep pass. Wrapped in an advisory lock so
// multi-instance deployments never duplicate work. Returns nil if another
// instance is currently sweeping.
func sweepOnce(ctx context.Context, db *sql.DB, store storage.FileStore) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	var locked bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, orphanLockID).Scan(&locked); err != nil {
		return err
	}
	if !locked {
		log.Println("orphan sweep: another instance holds the lock; skipping")
		return nil
	}
	defer func() {
		if _, err := conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, orphanLockID); err != nil {
			log.Printf("orphan sweep: release lock: %v", err)
		}
	}()

	objects, err := store.ListWithPrefix(ctx, "projects/")
	if err != nil {
		return err
	}
	if len(objects) == 0 {
		return nil
	}

	known, err := loadKnownStorageKeys(ctx, conn)
	if err != nil {
		return err
	}

	purged := 0
	for _, key := range orphans(objects, known, time.Now()) {
		if err := store.Delete(ctx, key); err != nil {
			log.Printf("orphan sweep: delete %s: %v", key, err)
			continue
		}
		purged++
	}
	if purged > 0 {
		log.Printf("orphan sweep: purged %d orphaned object(s)", purged)
	}
	return nil
}

// orphans is the pure filter at the heart of the sweep: an object is an
// orphan if it has no DB row *and* its last-modified timestamp is at least
// OrphanGracePeriod in the past.
func orphans(objects []storage.ObjectInfo, known map[string]struct{}, now time.Time) []string {
	cutoff := now.Add(-OrphanGracePeriod)
	var keys []string
	for _, obj := range objects {
		if _, ok := known[obj.Key]; ok {
			continue
		}
		if obj.LastModified.After(cutoff) {
			continue
		}
		keys = append(keys, obj.Key)
	}
	return keys
}

// loadKnownStorageKeys returns the set of storage_key values currently in the
// attachments table. Keys not in this set are candidates for deletion.
func loadKnownStorageKeys(ctx context.Context, conn *sql.Conn) (map[string]struct{}, error) {
	rows, err := conn.QueryContext(ctx, `SELECT storage_key FROM attachments`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]struct{}{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out[k] = struct{}{}
	}
	return out, rows.Err()
}

