package data

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
)

type AttachmentSource string

const (
	AttachmentSourceUpload       AttachmentSource = "upload"
	AttachmentSourceGooglePhotos AttachmentSource = "google_photos"
)

type Attachment struct {
	ID          string           `json:"id"`
	ProjectID   string           `json:"project_id"`
	ItemID      *string          `json:"item_id"`
	StorageKey  string           `json:"-"`
	Filename    string           `json:"filename"`
	ContentType string           `json:"content_type"`
	SizeBytes   int64            `json:"size_bytes"`
	Source      AttachmentSource `json:"source"`
	UploaderID  string           `json:"uploader_id"`
	CreatedAt   time.Time        `json:"created_at"`
}

const attachmentColumns = `id, project_id, item_id, storage_key, filename,
	content_type, size_bytes, source, uploader_id, created_at`

func scanAttachment(row interface {
	Scan(dest ...any) error
}) (*Attachment, error) {
	var a Attachment
	if err := row.Scan(
		&a.ID, &a.ProjectID, &a.ItemID, &a.StorageKey, &a.Filename,
		&a.ContentType, &a.SizeBytes, &a.Source, &a.UploaderID, &a.CreatedAt,
	); err != nil {
		return nil, err
	}
	return &a, nil
}

// CreateAttachmentParams gathers the fields needed to record an attachment.
// Mirrors the column set so the handler doesn't have to remember the order.
type CreateAttachmentParams struct {
	ProjectID   string
	ItemID      *string
	StorageKey  string
	Filename    string
	ContentType string
	SizeBytes   int64
	Source      AttachmentSource
	UploaderID  string
}

// CreateAttachment inserts a new row and returns the canonical record.
func CreateAttachment(ctx context.Context, tx *sql.Tx, p CreateAttachmentParams) (*Attachment, error) {
	row := tx.QueryRowContext(ctx, `
		INSERT INTO attachments (project_id, item_id, storage_key, filename, content_type, size_bytes, source, uploader_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+attachmentColumns,
		p.ProjectID, p.ItemID, p.StorageKey, p.Filename, p.ContentType, p.SizeBytes, p.Source, p.UploaderID)
	return scanAttachment(row)
}

// GetAttachmentByID returns the attachment with the given ID, or sql.ErrNoRows
// if not found.
func GetAttachmentByID(ctx context.Context, db *sql.DB, id string) (*Attachment, error) {
	row := db.QueryRowContext(ctx,
		`SELECT `+attachmentColumns+` FROM attachments WHERE id = $1`, id)
	return scanAttachment(row)
}

// ListAttachmentsForItem returns all attachments tied to the given item,
// oldest first (so the UI can display them in upload order).
func ListAttachmentsForItem(ctx context.Context, db *sql.DB, itemID string) ([]Attachment, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT `+attachmentColumns+` FROM attachments WHERE item_id = $1 ORDER BY created_at ASC`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	attachments := []Attachment{}
	for rows.Next() {
		a, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		attachments = append(attachments, *a)
	}
	return attachments, rows.Err()
}

// ListAttachmentsForItems batches the per-item lookup so listing items in a
// project doesn't issue N+1 queries.
func ListAttachmentsForItems(ctx context.Context, db *sql.DB, itemIDs []string) (map[string][]Attachment, error) {
	out := make(map[string][]Attachment, len(itemIDs))
	if len(itemIDs) == 0 {
		return out, nil
	}
	rows, err := db.QueryContext(ctx,
		`SELECT `+attachmentColumns+` FROM attachments WHERE item_id = ANY($1) ORDER BY created_at ASC`,
		pq.Array(itemIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		a, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		if a.ItemID != nil {
			out[*a.ItemID] = append(out[*a.ItemID], *a)
		}
	}
	return out, rows.Err()
}

// DeleteAttachment removes the row and returns the storage_key so the handler
// can purge the underlying object. Returns ErrNotFound if no row matched.
func DeleteAttachment(ctx context.Context, tx *sql.Tx, id string) (string, error) {
	var storageKey string
	err := tx.QueryRowContext(ctx,
		`DELETE FROM attachments WHERE id = $1 RETURNING storage_key`, id).Scan(&storageKey)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrAttachmentNotFound
	}
	if err != nil {
		return "", err
	}
	return storageKey, nil
}

// ErrAttachmentNotFound is returned by DeleteAttachment when nothing matched.
var ErrAttachmentNotFound = errors.New("attachment not found")
