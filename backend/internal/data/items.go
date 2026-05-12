package data

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type ItemKind string

const (
	ItemKindBrainstorm ItemKind = "brainstorm"
	ItemKindTask       ItemKind = "task"
)

type ItemStatus string

const (
	ItemStatusOpen       ItemStatus = "open"
	ItemStatusInProgress ItemStatus = "in_progress"
	ItemStatusDone       ItemStatus = "done"
	ItemStatusArchived   ItemStatus = "archived"
)

func IsValidItemKind(k string) bool {
	switch ItemKind(k) {
	case ItemKindBrainstorm, ItemKindTask:
		return true
	}
	return false
}

func IsValidItemStatus(s string) bool {
	switch ItemStatus(s) {
	case ItemStatusOpen, ItemStatusInProgress, ItemStatusDone, ItemStatusArchived:
		return true
	}
	return false
}

type Item struct {
	ID         string     `json:"id"`
	ProjectID  string     `json:"project_id"`
	Sequence   int        `json:"sequence"`
	Kind       ItemKind   `json:"kind"`
	Status     ItemStatus `json:"status"`
	Title      string     `json:"title"`
	Body       *string    `json:"body"`
	Position   float64    `json:"position"`
	CreatorID  string     `json:"creator_id"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

const itemColumns = `id, project_id, sequence, kind, status, title, body, position,
	creator_id, created_at, updated_at`

func scanItem(row interface {
	Scan(dest ...any) error
}) (*Item, error) {
	var i Item
	if err := row.Scan(
		&i.ID, &i.ProjectID, &i.Sequence, &i.Kind, &i.Status, &i.Title, &i.Body,
		&i.Position, &i.CreatorID, &i.CreatedAt, &i.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &i, nil
}

// CreateItem claims the next per-project sequence number atomically and
// inserts the item in a single transaction.
func CreateItem(ctx context.Context, db *sql.DB, projectID, creatorID string, kind ItemKind, title string, body *string) (*Item, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var seq int
	err = tx.QueryRowContext(ctx, `
		UPDATE project_item_sequences
		SET next_sequence = next_sequence + 1
		WHERE project_id = $1
		RETURNING next_sequence - 1
	`, projectID).Scan(&seq)
	if err != nil {
		return nil, fmt.Errorf("claim sequence: %w", err)
	}

	row := tx.QueryRowContext(ctx, `
		INSERT INTO items (project_id, sequence, kind, title, body, creator_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+itemColumns,
		projectID, seq, kind, title, body, creatorID)

	item, err := scanItem(row)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return item, nil
}

// GetItemBySequence fetches an item by its per-project `#N` reference.
func GetItemBySequence(ctx context.Context, db *sql.DB, projectID string, sequence int) (*Item, error) {
	row := db.QueryRowContext(ctx,
		`SELECT `+itemColumns+` FROM items WHERE project_id = $1 AND sequence = $2`,
		projectID, sequence)
	return scanItem(row)
}

// ListItemsForProject returns all items in a project, ordered by position DESC
// (newest first by default). Optional kind/status filters; pass "" to skip either.
func ListItemsForProject(ctx context.Context, db *sql.DB, projectID, kind, status string) ([]Item, error) {
	clauses := []string{"project_id = $1"}
	args := []any{projectID}

	if kind != "" {
		args = append(args, kind)
		clauses = append(clauses, fmt.Sprintf("kind = $%d", len(args)))
	}
	if status != "" {
		args = append(args, status)
		clauses = append(clauses, fmt.Sprintf("status = $%d", len(args)))
	}

	query := `SELECT ` + itemColumns + ` FROM items
		WHERE ` + strings.Join(clauses, " AND ") + `
		ORDER BY position DESC`

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Item{}
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

// ItemPatch carries optional fields for UpdateItem. nil = leave alone.
// For Body and Position, pass a non-nil pointer to clear/set; the inner value
// is what gets written.
type ItemPatch struct {
	Title    *string
	Body     **string // pointer-to-pointer so nil-inside = clear, vs no-write
	Kind     *ItemKind
	Status   *ItemStatus
	Position *float64
}

// UpdateItem applies a patch and bumps updated_at. Returns the updated row.
func UpdateItem(ctx context.Context, db *sql.DB, itemID string, patch ItemPatch) (*Item, error) {
	sets := []string{"updated_at = NOW()"}
	args := []any{}

	add := func(col string, val any) {
		args = append(args, val)
		sets = append(sets, fmt.Sprintf("%s = $%d", col, len(args)))
	}

	if patch.Title != nil {
		add("title", *patch.Title)
	}
	if patch.Body != nil {
		add("body", *patch.Body) // *patch.Body is *string; nil = clears
	}
	if patch.Kind != nil {
		add("kind", *patch.Kind)
	}
	if patch.Status != nil {
		add("status", *patch.Status)
	}
	if patch.Position != nil {
		add("position", *patch.Position)
	}

	args = append(args, itemID)
	query := `UPDATE items SET ` + strings.Join(sets, ", ") +
		fmt.Sprintf(` WHERE id = $%d RETURNING `, len(args)) + itemColumns

	row := db.QueryRowContext(ctx, query, args...)
	return scanItem(row)
}

// DeleteItem removes an item. Returns sql.ErrNoRows if the item didn't exist
func DeleteItem(ctx context.Context, db *sql.DB, itemID string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM items WHERE id = $1`, itemID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}
