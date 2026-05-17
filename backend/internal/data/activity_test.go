package data_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"trove/backend/internal/data"
	"trove/backend/internal/data/datatest"
)

// logActivity inserts one event through a WithRetry tx and returns the row.
func logActivity(t *testing.T, db *sql.DB, in data.ActivityInput) *data.Activity {
	t.Helper()
	var a *data.Activity
	if err := data.WithRetry(context.Background(), db, func(tx *sql.Tx) error {
		var e error
		a, e = data.LogActivity(context.Background(), tx, in)
		return e
	}); err != nil {
		t.Fatalf("logActivity: %v", err)
	}
	return a
}

// txDo runs a single mutation in a WithRetry tx, failing the test on error.
func txDo(t *testing.T, db *sql.DB, fn func(tx *sql.Tx) error) {
	t.Helper()
	if err := data.WithRetry(context.Background(), db, fn); err != nil {
		t.Fatalf("txDo: %v", err)
	}
}

func TestLogActivity_RowShape(t *testing.T) {
	db := datatest.OpenTestDB(t)
	user := datatest.SeedUser(t, db)
	project := datatest.SeedProject(t, db, user)
	item := datatest.SeedItem(t, db, project.ID, user)

	got := logActivity(t, db, data.ActivityInput{
		ProjectID: project.ID,
		ItemID:    &item.ID,
		ActorID:   user,
		Action:    data.ActivityItemCreated,
		Payload: map[string]any{
			"item": map[string]any{"seq": item.Sequence, "title": item.Title},
		},
	})

	if got.ID == "" {
		t.Error("ID empty")
	}
	if got.ProjectID != project.ID {
		t.Errorf("ProjectID = %q, want %q", got.ProjectID, project.ID)
	}
	if got.ItemID == nil || *got.ItemID != item.ID {
		t.Errorf("ItemID = %v, want %q", got.ItemID, item.ID)
	}
	if got.ActorID != user {
		t.Errorf("ActorID = %q, want %q", got.ActorID, user)
	}
	if got.Action != data.ActivityItemCreated {
		t.Errorf("Action = %q, want %q", got.Action, data.ActivityItemCreated)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}

	var p struct {
		Item struct {
			Seq   int    `json:"seq"`
			Title string `json:"title"`
		} `json:"item"`
	}
	if err := json.Unmarshal(got.Payload, &p); err != nil {
		t.Fatalf("payload not valid JSON: %v", err)
	}
	if p.Item.Seq != item.Sequence || p.Item.Title != item.Title {
		t.Errorf("payload snapshot = %+v, want seq=%d title=%q", p.Item, item.Sequence, item.Title)
	}
}

func TestLogActivity_NilPayloadIsEmptyObject(t *testing.T) {
	db := datatest.OpenTestDB(t)
	user := datatest.SeedUser(t, db)
	project := datatest.SeedProject(t, db, user)

	got := logActivity(t, db, data.ActivityInput{
		ProjectID: project.ID,
		ActorID:   user,
		Action:    data.ActivityProjectCreated,
	})
	if string(got.Payload) != "{}" {
		t.Errorf("nil payload stored as %q, want %q", string(got.Payload), "{}")
	}
	if got.ItemID != nil {
		t.Errorf("ItemID = %v, want nil", got.ItemID)
	}
}

// TestWithRetry_Atomicity is the headline guarantee: the mutation and its
// activity row commit or roll back as one unit.
func TestWithRetry_Atomicity(t *testing.T) {
	db := datatest.OpenTestDB(t)
	ctx := context.Background()
	user := datatest.SeedUser(t, db)
	project := datatest.SeedProject(t, db, user)
	item := datatest.SeedItem(t, db, project.ID, user)

	t.Run("commit: both land", func(t *testing.T) {
		newTitle := "renamed in tx"
		err := data.WithRetry(ctx, db, func(tx *sql.Tx) error {
			if _, err := data.UpdateItem(ctx, tx, item.ID, data.ItemPatch{Title: &newTitle}); err != nil {
				return err
			}
			_, err := data.LogActivity(ctx, tx, data.ActivityInput{
				ProjectID: project.ID,
				ItemID:    &item.ID,
				ActorID:   user,
				Action:    data.ActivityItemUpdated,
				Payload:   map[string]any{"diff": map[string]any{"title": map[string]string{"old": item.Title, "new": newTitle}}},
			})
			return err
		})
		if err != nil {
			t.Fatalf("WithRetry: %v", err)
		}

		got, err := data.GetItemBySequence(ctx, db, project.ID, item.Sequence)
		if err != nil {
			t.Fatalf("GetItemBySequence: %v", err)
		}
		if got.Title != newTitle {
			t.Errorf("title = %q, want %q (mutation should have committed)", got.Title, newTitle)
		}
		if n := countActivity(t, db, project.ID); n != 1 {
			t.Errorf("activity rows = %d, want 1", n)
		}
	})

	t.Run("rollback: neither lands", func(t *testing.T) {
		boom := errors.New("boom")
		bad := "should not persist"
		err := data.WithRetry(ctx, db, func(tx *sql.Tx) error {
			if _, err := data.UpdateItem(ctx, tx, item.ID, data.ItemPatch{Title: &bad}); err != nil {
				return err
			}
			if _, err := data.LogActivity(ctx, tx, data.ActivityInput{
				ProjectID: project.ID, ItemID: &item.ID, ActorID: user, Action: data.ActivityItemUpdated,
			}); err != nil {
				return err
			}
			return boom // force rollback after both writes
		})
		if !errors.Is(err, boom) {
			t.Fatalf("WithRetry err = %v, want boom", err)
		}

		got, err := data.GetItemBySequence(ctx, db, project.ID, item.Sequence)
		if err != nil {
			t.Fatalf("GetItemBySequence: %v", err)
		}
		if got.Title == bad {
			t.Error("item title persisted despite rolled-back tx")
		}
		// Only the prior subtest's row should exist; the rolled-back LogActivity must not.
		if n := countActivity(t, db, project.ID); n != 1 {
			t.Errorf("activity rows = %d, want 1 (rolled-back insert must not persist)", n)
		}
	})
}

func TestItemDelete_KeepsActivity_SetsItemNull(t *testing.T) {
	db := datatest.OpenTestDB(t)
	ctx := context.Background()
	user := datatest.SeedUser(t, db)
	project := datatest.SeedProject(t, db, user)
	item := datatest.SeedItem(t, db, project.ID, user)

	logActivity(t, db, data.ActivityInput{
		ProjectID: project.ID,
		ItemID:    &item.ID,
		ActorID:   user,
		Action:    data.ActivityItemCreated,
		Payload:   map[string]any{"item": map[string]any{"seq": item.Sequence, "title": item.Title}},
	})

	txDo(t, db, func(tx *sql.Tx) error { return data.DeleteItem(ctx, tx, item.ID) })

	// Item gone…
	if _, err := data.GetItemBySequence(ctx, db, project.ID, item.Sequence); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("GetItemBySequence after delete: err = %v, want sql.ErrNoRows", err)
	}

	// …but its history survives, detached, snapshot intact.
	var itemID *string
	var payload []byte
	if err := db.QueryRowContext(ctx,
		`SELECT item_id, payload FROM activity WHERE project_id = $1`, project.ID,
	).Scan(&itemID, &payload); err != nil {
		t.Fatalf("query activity after item delete: %v", err)
	}
	if itemID != nil {
		t.Errorf("item_id = %v, want NULL after item delete (ON DELETE SET NULL)", *itemID)
	}
	var p struct {
		Item struct {
			Seq   int    `json:"seq"`
			Title string `json:"title"`
		} `json:"item"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.Item.Seq != item.Sequence || p.Item.Title != item.Title {
		t.Errorf("snapshot lost: %+v, want seq=%d title=%q", p.Item, item.Sequence, item.Title)
	}
}

func TestProjectDelete_CascadesActivity(t *testing.T) {
	db := datatest.OpenTestDB(t)
	ctx := context.Background()
	user := datatest.SeedUser(t, db)
	project := datatest.SeedProject(t, db, user)
	item := datatest.SeedItem(t, db, project.ID, user)

	logActivity(t, db, data.ActivityInput{
		ProjectID: project.ID, ItemID: &item.ID, ActorID: user, Action: data.ActivityItemCreated,
	})

	txDo(t, db, func(tx *sql.Tx) error { return data.DeleteProject(ctx, tx, project.ID) })
	if n := countActivity(t, db, project.ID); n != 0 {
		t.Errorf("activity rows after project delete = %d, want 0 (ON DELETE CASCADE)", n)
	}
}

func countActivity(t *testing.T, db *sql.DB, projectID string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM activity WHERE project_id = $1`, projectID).Scan(&n); err != nil {
		t.Fatalf("countActivity: %v", err)
	}
	return n
}

// insertActivityAt inserts a row with an explicit created_at so ordering /
// keyset tests are deterministic (LogActivity defaults created_at to NOW()).
func insertActivityAt(t *testing.T, db *sql.DB, projectID string, itemID *string, actorID string, action data.ActivityAction, at time.Time) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO activity (project_id, item_id, actor_id, action, payload, created_at)
		VALUES ($1, $2, $3, $4, '{}', $5)
	`, projectID, itemID, actorID, action, at); err != nil {
		t.Fatalf("insertActivityAt: %v", err)
	}
}

func TestListActivity_KeysetPagination(t *testing.T) {
	db := datatest.OpenTestDB(t)
	ctx := context.Background()
	user := datatest.SeedUser(t, db)
	project := datatest.SeedProject(t, db, user)

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ { // t0 oldest … t4 newest
		insertActivityAt(t, db, project.ID, nil, user, data.ActivityProjectUpdated, base.Add(time.Duration(i)*time.Minute))
	}

	page1, err := data.ListActivity(ctx, db, data.ActivityFilter{ProjectID: project.ID, Limit: 2})
	if err != nil {
		t.Fatalf("ListActivity page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1 len = %d, want 2", len(page1))
	}
	if !page1[0].CreatedAt.After(page1[1].CreatedAt) {
		t.Error("page not newest-first")
	}

	cursor := &data.ActivityCursor{CreatedAt: page1[1].CreatedAt, ID: page1[1].ID}
	page2, err := data.ListActivity(ctx, db, data.ActivityFilter{ProjectID: project.ID, Limit: 2, Before: cursor})
	if err != nil {
		t.Fatalf("ListActivity page2: %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("page2 len = %d, want 2", len(page2))
	}
	for _, a := range page2 {
		if !a.CreatedAt.Before(cursor.CreatedAt) {
			t.Errorf("page2 row %s not strictly older than cursor", a.ID)
		}
	}
	// No overlap between pages.
	seen := map[string]bool{page1[0].ID: true, page1[1].ID: true}
	for _, a := range page2 {
		if seen[a.ID] {
			t.Errorf("row %s appeared on both pages", a.ID)
		}
	}
}

func TestListActivity_Filters(t *testing.T) {
	db := datatest.OpenTestDB(t)
	ctx := context.Background()
	owner := datatest.SeedUser(t, db)
	other := datatest.SeedUser(t, db)
	project := datatest.SeedProject(t, db, owner)
	itemA := datatest.SeedItem(t, db, project.ID, owner)
	itemB := datatest.SeedItem(t, db, project.ID, owner)

	base := time.Date(2026, 2, 1, 9, 0, 0, 0, time.UTC)
	insertActivityAt(t, db, project.ID, &itemA.ID, owner, data.ActivityItemUpdated, base)
	insertActivityAt(t, db, project.ID, &itemA.ID, owner, data.ActivityItemTagAdded, base.Add(1*time.Minute))
	insertActivityAt(t, db, project.ID, &itemB.ID, other, data.ActivityItemUpdated, base.Add(2*time.Minute))
	insertActivityAt(t, db, project.ID, nil, owner, data.ActivityProjectUpdated, base.Add(3*time.Minute))

	t.Run("by item", func(t *testing.T) {
		got, err := data.ListActivity(ctx, db, data.ActivityFilter{ProjectID: project.ID, ItemID: &itemA.ID})
		if err != nil {
			t.Fatalf("ListActivity: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2 (itemA only)", len(got))
		}
		for _, a := range got {
			if a.ItemID == nil || *a.ItemID != itemA.ID {
				t.Errorf("row item_id = %v, want %q", a.ItemID, itemA.ID)
			}
		}
	})

	t.Run("by actor", func(t *testing.T) {
		got, err := data.ListActivity(ctx, db, data.ActivityFilter{ProjectID: project.ID, ActorID: &other})
		if err != nil {
			t.Fatalf("ListActivity: %v", err)
		}
		if len(got) != 1 || got[0].ActorID != other {
			t.Fatalf("got %d rows for actor %s, want 1", len(got), other)
		}
	})

	t.Run("by action set", func(t *testing.T) {
		got, err := data.ListActivity(ctx, db, data.ActivityFilter{
			ProjectID: project.ID,
			Actions:   []data.ActivityAction{data.ActivityItemUpdated},
		})
		if err != nil {
			t.Fatalf("ListActivity: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2 (item.updated only)", len(got))
		}
		for _, a := range got {
			if a.Action != data.ActivityItemUpdated {
				t.Errorf("action = %q, want item.updated", a.Action)
			}
		}
	})
}
