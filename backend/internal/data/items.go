package data

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
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
	ID        string     `json:"id"`
	ProjectID string     `json:"project_id"`
	Sequence  int        `json:"sequence"`
	Kind      ItemKind   `json:"kind"`
	Status    ItemStatus `json:"status"`
	Title     string     `json:"title"`
	Body      *string    `json:"body"`
	Position  float64    `json:"position"`
	CreatorID string     `json:"creator_id"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	Tags      []Tag      `json:"tags"`
}

type ItemFilter struct {
	Kind     string
	Status   string
	TagSlugs []string
	TagMode  string
}

const itemColumns = `id, project_id, sequence, kind, status, title, body, position,
	creator_id, created_at, updated_at`

const itemColumnsI = `i.id, i.project_id, i.sequence, i.kind, i.status, i.title, i.body,
	i.position, i.creator_id, i.created_at, i.updated_at`

func scanItem(row interface {
	Scan(dest ...any) error
}) (*Item, error) {
	i := Item{Tags: []Tag{}}
	if err := row.Scan(
		&i.ID, &i.ProjectID, &i.Sequence, &i.Kind, &i.Status, &i.Title, &i.Body,
		&i.Position, &i.CreatorID, &i.CreatedAt, &i.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &i, nil
}

// CreateItem claims the next per-project sequence number and inserts the item.
// The two statements must run in one transaction (call inside WithRetry) so a
// concurrent create can't be handed the same sequence and the activity row
// commits in the same unit.
func CreateItem(ctx context.Context, tx *sql.Tx, projectID, creatorID string, kind ItemKind, title string, body *string) (*Item, error) {
	var seq int
	err := tx.QueryRowContext(ctx, `
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

	return scanItem(row)
}

// GetItemBySequence fetches an item by its per-project `#N` reference.
func GetItemBySequence(ctx context.Context, db *sql.DB, projectID string, sequence int) (*Item, error) {
	row := db.QueryRowContext(ctx,
		`SELECT `+itemColumns+` FROM items WHERE project_id = $1 AND sequence = $2`,
		projectID, sequence)
	return scanItem(row)
}

// ListItemsForProject returns items in a project, ordered by position DESC
// (newest first by default). The filter struct is optional — zero-value means
// "no filtering". For tag filters, TagMode="and" requires items to have all
// listed tags; "or" requires any.
func ListItemsForProject(ctx context.Context, db *sql.DB, projectID string, filter ItemFilter) ([]Item, error) {
	clauses := []string{"i.project_id = $1"}
	args := []any{projectID}

	if filter.Kind != "" {
		args = append(args, filter.Kind)
		clauses = append(clauses, fmt.Sprintf("i.kind = $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		clauses = append(clauses, fmt.Sprintf("i.status = $%d", len(args)))
	}
	if len(filter.TagSlugs) > 0 {
		args = append(args, pq.Array(filter.TagSlugs))
		tagsArgIdx := len(args)
		if filter.TagMode == "or" {
			clauses = append(clauses, fmt.Sprintf(`EXISTS (
				SELECT 1 FROM item_tags it
				JOIN tags t ON t.id = it.tag_id
				WHERE it.item_id = i.id AND t.slug = ANY($%d)
			)`, tagsArgIdx))
		} else {
			// Default AND: item must have every listed slug.
			clauses = append(clauses, fmt.Sprintf(`(
				SELECT COUNT(DISTINCT t.slug) FROM tags t
				JOIN item_tags it ON it.tag_id = t.id
				WHERE it.item_id = i.id AND t.slug = ANY($%d)
			) = cardinality($%d)`, tagsArgIdx, tagsArgIdx))
		}
	}

	query := `SELECT ` + itemColumnsI + ` FROM items i
		WHERE ` + strings.Join(clauses, " AND ") + `
		ORDER BY i.position DESC`

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
func UpdateItem(ctx context.Context, tx *sql.Tx, itemID string, patch ItemPatch) (*Item, error) {
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

	row := tx.QueryRowContext(ctx, query, args...)
	return scanItem(row)
}

// DeleteItem removes an item. Returns sql.ErrNoRows if the item didn't exist
func DeleteItem(ctx context.Context, tx *sql.Tx, itemID string) error {
	res, err := tx.ExecContext(ctx, `DELETE FROM items WHERE id = $1`, itemID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}
