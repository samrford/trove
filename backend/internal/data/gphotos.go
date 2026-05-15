package data

import (
	"context"
	"database/sql"
	"errors"
)

// GPhotosImportTarget records which Trove item a Google Photos import job is
// destined for. Inserted at StartImport, consumed by the sink when each photo
// arrives.
type GPhotosImportTarget struct {
	JobID     string
	ProjectID string
	ItemID    string
	UserID    string
}

// CreateGPhotosImportTarget inserts a target row. If the job_id already
// exists (re-issued StartImport for a session, say), the row stays as-is —
// the worker will keep delivering to the original destination.
func CreateGPhotosImportTarget(ctx context.Context, db *sql.DB, t GPhotosImportTarget) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO gphotos_import_targets (job_id, project_id, item_id, user_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (job_id) DO NOTHING
	`, t.JobID, t.ProjectID, t.ItemID, t.UserID)
	return err
}

// GetGPhotosImportTarget fetches the destination for a job. Returns
// ErrGPhotosTargetNotFound if no row matches — typically means the picker
// library is delivering for a job we don't know about (shouldn't happen, but
// the sink should fail loudly rather than silently drop the photo).
func GetGPhotosImportTarget(ctx context.Context, db *sql.DB, jobID string) (*GPhotosImportTarget, error) {
	var t GPhotosImportTarget
	err := db.QueryRowContext(ctx, `
		SELECT job_id, project_id, item_id, user_id
		FROM gphotos_import_targets
		WHERE job_id = $1
	`, jobID).Scan(&t.JobID, &t.ProjectID, &t.ItemID, &t.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrGPhotosTargetNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ErrGPhotosTargetNotFound is returned when a job_id has no destination row.
var ErrGPhotosTargetNotFound = errors.New("google photos import target not found")
