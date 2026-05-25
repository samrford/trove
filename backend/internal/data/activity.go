package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

type ActivityAction string

const (
	ActivityItemCreated       ActivityAction = "item.created"
	ActivityItemUpdated       ActivityAction = "item.updated"
	ActivityItemDeleted       ActivityAction = "item.deleted"
	ActivityItemTagAdded      ActivityAction = "item.tag_added"
	ActivityItemTagRemoved    ActivityAction = "item.tag_removed"
	ActivityAttachmentAdded   ActivityAction = "attachment.added"
	ActivityAttachmentRemoved ActivityAction = "attachment.removed"
	ActivityProjectCreated    ActivityAction = "project.created"
	ActivityProjectUpdated    ActivityAction = "project.updated"
	ActivityProjectDeleted    ActivityAction = "project.deleted"
	// ActivityNote is reserved for the future MCP `log` tool — actor-authored
	// free text. Declared so the enum needs no later migration; no write path
	// emits it yet.
	ActivityNote ActivityAction = "note"
)

// Activity is one row of the event log. Payload is a self-contained JSON blob
// (item {seq,title,kind} snapshot + field diffs / relation detail) so the feed
// and the AI never need a join — and a row stays readable after its item is
// deleted (item_id goes NULL, the snapshot remains).
type Activity struct {
	ID        string          `json:"id"`
	ProjectID string          `json:"project_id"`
	ItemID    *string         `json:"item_id"`
	ActorID   string          `json:"actor_id"`
	Action    ActivityAction  `json:"action"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

// ActivityInput is what callers hand LogActivity. Payload is marshalled to
// JSONB; nil becomes '{}'.
type ActivityInput struct {
	ProjectID string
	ItemID    *string
	ActorID   string
	Action    ActivityAction
	Payload   any
}

const activityColumns = `id, project_id, item_id, actor_id, action, payload, created_at`

func scanActivity(row interface {
	Scan(dest ...any) error
}) (*Activity, error) {
	var a Activity
	var payload []byte
	if err := row.Scan(
		&a.ID, &a.ProjectID, &a.ItemID, &a.ActorID, &a.Action, &payload, &a.CreatedAt,
	); err != nil {
		return nil, err
	}
	a.Payload = json.RawMessage(payload)
	return &a, nil
}

// LogActivity inserts one event. Pass the *same* tx as the mutation it
// records (the WithRetry tx) so state and activity commit as one unit.
func LogActivity(ctx context.Context, tx *sql.Tx, in ActivityInput) (*Activity, error) {
	payload := []byte("{}")
	if in.Payload != nil {
		b, err := json.Marshal(in.Payload)
		if err != nil {
			return nil, fmt.Errorf("marshal activity payload: %w", err)
		}
		payload = b
	}
	row := tx.QueryRowContext(ctx, `
		INSERT INTO activity (project_id, item_id, actor_id, action, payload)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+activityColumns,
		in.ProjectID, in.ItemID, in.ActorID, in.Action, string(payload))
	return scanActivity(row)
}

// ActivityCursor is the keyset position for pagination — the (created_at, id)
// of the last row a client has seen. Doubles as the SSE catch-up cursor.
type ActivityCursor struct {
	CreatedAt time.Time
	ID        string
}

// ActivityFilter scopes a ListActivity query. ProjectID is required; the rest
// are optional. Before paginates to rows strictly older than the cursor.
type ActivityFilter struct {
	ProjectID string
	ItemID    *string
	ActorID   *string
	Actions   []ActivityAction
	Before    *ActivityCursor
	Limit     int
}

const (
	defaultActivityLimit = 50
	maxActivityLimit     = 200
)

// ClampActivityLimit applies the same bound ListActivity uses internally —
// exported so the endpoint can size its "next" cursor against the real page.
func ClampActivityLimit(n int) int {
	if n <= 0 {
		return defaultActivityLimit
	}
	if n > maxActivityLimit {
		return maxActivityLimit
	}
	return n
}

// ListActivity returns a project's events newest-first, keyset-paginated. The
// (created_at, id) row-value comparison rides activity_project_created_idx.
func ListActivity(ctx context.Context, db *sql.DB, f ActivityFilter) ([]Activity, error) {
	clauses := []string{"project_id = $1"}
	args := []any{f.ProjectID}

	if f.ItemID != nil {
		args = append(args, *f.ItemID)
		clauses = append(clauses, fmt.Sprintf("item_id = $%d", len(args)))
	}
	if f.ActorID != nil {
		args = append(args, *f.ActorID)
		clauses = append(clauses, fmt.Sprintf("actor_id = $%d", len(args)))
	}
	if len(f.Actions) > 0 {
		strs := make([]string, len(f.Actions))
		for i, a := range f.Actions {
			strs[i] = string(a)
		}
		args = append(args, pq.Array(strs))
		clauses = append(clauses, fmt.Sprintf("action = ANY($%d)", len(args)))
	}
	if f.Before != nil {
		args = append(args, f.Before.CreatedAt, f.Before.ID)
		clauses = append(clauses, fmt.Sprintf("(created_at, id) < ($%d, $%d)", len(args)-1, len(args)))
	}

	args = append(args, ClampActivityLimit(f.Limit))

	query := `SELECT ` + activityColumns + ` FROM activity WHERE ` +
		strings.Join(clauses, " AND ") +
		fmt.Sprintf(` ORDER BY created_at DESC, id DESC LIMIT $%d`, len(args))

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Activity{}
	for rows.Next() {
		a, err := scanActivity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// GetActivityByID fetches one event.
func GetActivityByID(ctx context.Context, db *sql.DB, id string) (*Activity, error) {
	row := db.QueryRowContext(ctx,
		`SELECT `+activityColumns+` FROM activity WHERE id = $1`, id)
	return scanActivity(row)
}

// ActivitySince returns events strictly newer than the cursor (exclusive),
// *oldest-first*, across the given projects, bounded by the clamped limit.
// A zero cursor(empty ID) means "from the beginning", a zero limit means
// "the default page size".
func ActivitySince(ctx context.Context, db *sql.DB, projectIDs []string, after ActivityCursor, limit int) ([]Activity, error) {
	out := []Activity{}
	if len(projectIDs) == 0 {
		return out, nil
	}
	clauses := []string{"project_id = ANY($1)"}
	args := []any{pq.Array(projectIDs)}
	if after.ID != "" {
		args = append(args, after.CreatedAt, after.ID)
		clauses = append(clauses, fmt.Sprintf("(created_at, id) > ($%d, $%d)", len(args)-1, len(args)))
	}
	args = append(args, ClampActivityLimit(limit))

	query := `SELECT ` + activityColumns + ` FROM activity WHERE ` +
		strings.Join(clauses, " AND ") +
		fmt.Sprintf(` ORDER BY created_at ASC, id ASC LIMIT $%d`, len(args))

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		a, err := scanActivity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}
