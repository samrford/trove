// Package events is the SSE fan-out layer: it turns committed activity rows
// (delivered via Postgres NOTIFY) into the typed event messages.
// Build is the single source of event construction — the live hub
// and the reconnect catch-up path both go through it, so they can't diverge.
package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"trove/backend/internal/data"
)

// Envelope is the slim pg_notify payload.
// Never the event body — the hub re-reads the full row by ActivityID.
type Envelope struct {
	ActivityID string  `json:"activity_id"`
	ProjectID  string  `json:"project_id"`
	ItemID     *string `json:"item_id"`
	Action     string  `json:"action"`
	CreatedAt  string  `json:"created_at"`
}

// Message is one rendered SSE frame: a named event + JSON data + the cursor
// that doubles as the SSE id (a client's Last-Event-ID is our catch-up
// cursor). ProjectID drives fan-out filtering and is not serialised.
type Message struct {
	Event     string
	Data      json.RawMessage
	Cursor    string
	ProjectID string
}

const (
	EventActivityAdded  = "activity.added"
	EventItemChanged    = "item.changed"
	EventItemDeleted    = "item.deleted"
	EventProjectChanged = "project.changed"
	EventResync         = "resync"
)

// ItemHydrator turns a bare data.Item into the JSON shape the front-end
// expects from REST endpoints (tags loaded, attachments signed). Wired at
// hub construction so the events package stays decoupled from handlers /
// storage — main.go supplies the implementation that reuses the REST
// handlers' wrapping. nil = ship the bare item (used in tests).
type ItemHydrator func(ctx context.Context, item *data.Item) (any, error)

// CursorOf / ParseCursor: the wire form is "<RFC3339Nano>|<uuid>" — one
// string for the SSE id / Last-Event-ID, decomposed into the (created_at,id)
// keyset the data layer paginates on.
func CursorOf(a *data.Activity) string {
	return a.CreatedAt.Format(time.RFC3339Nano) + "|" + a.ID
}

func ParseCursor(s string) (data.ActivityCursor, bool) {
	i := strings.LastIndex(s, "|")
	if i < 0 {
		return data.ActivityCursor{}, false
	}
	ts, err := time.Parse(time.RFC3339Nano, s[:i])
	id := s[i+1:]
	if err != nil || id == "" {
		return data.ActivityCursor{}, false
	}
	return data.ActivityCursor{CreatedAt: ts, ID: id}, true
}

// Build turns one activity row into the SSE messages it implies:
//
//   - always: activity.added (feeds the activity surfaces)
//   - item.deleted action      -> item.deleted {id,seq,project_id,actor_id}
//   - other item-scoped action -> item.changed {item: <full Item>, actor_id}
//   - project.created/updated  -> project.changed (the full Project)
//
// item.changed/item.deleted carry actor_id so the front-end can identify the
// actor's own echo (and skip the "changed elsewhere" affordance) directly.
//
// The item id comes from the self-contained payload snapshot (present even on
// item.deleted, where the row's item_id has gone NULL via FK SET NULL) — so
// live and catch-up reconstruct identically. item.changed/project.changed read
// the *current* row; if it's since gone (sql.ErrNoRows) only activity.added is
// emitted (the delete event carries the removal).
func Build(ctx context.Context, db *sql.DB, hydrate ItemHydrator, a *data.Activity) ([]Message, error) {
	body, err := json.Marshal(a)
	if err != nil {
		return nil, err
	}
	cur := CursorOf(a)
	msgs := []Message{{Event: EventActivityAdded, Data: body, Cursor: cur, ProjectID: a.ProjectID}}

	var snap struct {
		Item struct {
			ID  string `json:"id"`
			Seq int    `json:"seq"`
		} `json:"item"`
	}
	_ = json.Unmarshal(a.Payload, &snap)
	itemID := snap.Item.ID
	// Fall back to the row's item_id column when the payload snapshot predates
	// the `id` field (older rows replayed on catch-up). The column is NULL only
	// for item.deleted (FK SET NULL), where the snapshot is the sole source.
	if itemID == "" && a.ItemID != nil {
		itemID = *a.ItemID
	}

	switch a.Action {
	case data.ActivityItemDeleted:
		d, _ := json.Marshal(map[string]any{
			"id": itemID, "seq": snap.Item.Seq, "project_id": a.ProjectID,
			"actor_id": a.ActorID,
		})
		msgs = append(msgs, Message{Event: EventItemDeleted, Data: d, Cursor: cur, ProjectID: a.ProjectID})

	case data.ActivityItemCreated, data.ActivityItemUpdated,
		data.ActivityItemTagAdded, data.ActivityItemTagRemoved,
		data.ActivityAttachmentAdded, data.ActivityAttachmentRemoved:
		if itemID != "" {
			it, err := data.GetItemByID(ctx, db, itemID)
			switch {
			case err == nil:
				// Match REST payload shape (tags + signed attachments) so the
				// front-end's Item type is satisfied identically by REST and SSE.
				var payload any = it
				if hydrate != nil {
					if p, herr := hydrate(ctx, it); herr == nil {
						payload = p
					} else {
						log.Printf("sse hydrate item %s: %v", itemID, herr)
					}
				}
				// Wrap the item with its originating actor so the front-end can
				// suppress the "changed elsewhere" affordance on the actor's own
				// echo by comparing actor_id directly — no fragile correlation
				// with the sibling activity.added frame.
				d, _ := json.Marshal(map[string]any{"item": payload, "actor_id": a.ActorID})
				msgs = append(msgs, Message{Event: EventItemChanged, Data: d, Cursor: cur, ProjectID: a.ProjectID})
			case !errors.Is(err, sql.ErrNoRows):
				return nil, err
			}
		}

	case data.ActivityProjectCreated, data.ActivityProjectUpdated:
		p, err := data.GetProjectByID(ctx, db, a.ProjectID)
		switch {
		case err == nil:
			d, _ := json.Marshal(p)
			msgs = append(msgs, Message{Event: EventProjectChanged, Data: d, Cursor: cur, ProjectID: a.ProjectID})
		case !errors.Is(err, sql.ErrNoRows):
			return nil, err
		}
	}
	return msgs, nil
}
