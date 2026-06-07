package data_test

import (
	"context"
	"testing"

	"trove/backend/internal/data"
	"trove/backend/internal/data/datatest"
)

// ActivitySince is the SSE reconnect catch-up read: forward, oldest-first,
// strictly after the cursor, scoped to a project set, bounded.

func TestActivitySince(t *testing.T) {
	db := datatest.OpenTestDB(t)
	ctx := context.Background()
	user := datatest.SeedUser(t, db)
	p1 := datatest.SeedProject(t, db, user)
	p2 := datatest.SeedProject(t, db, user)

	// Six events: P1, P2, P1, P1, P2, P1 (interleaved across projects).
	for _, pid := range []string{p1.ID, p2.ID, p1.ID, p1.ID, p2.ID, p1.ID} {
		logActivity(t, db, data.ActivityInput{
			ProjectID: pid, ActorID: user, Action: data.ActivityItemCreated,
		})
	}

	zero := data.ActivityCursor{}

	// Canonical order across both projects, oldest-first.
	all, err := data.ActivitySince(ctx, db, []string{p1.ID, p2.ID}, zero, 50)
	if err != nil {
		t.Fatalf("ActivitySince all: %v", err)
	}
	if len(all) != 6 {
		t.Fatalf("want 6 rows, got %d", len(all))
	}
	for i := 1; i < len(all); i++ {
		prev, cur := all[i-1], all[i]
		if cur.CreatedAt.Before(prev.CreatedAt) ||
			(cur.CreatedAt.Equal(prev.CreatedAt) && cur.ID < prev.ID) {
			t.Fatalf("not ascending by (created_at,id) at %d", i)
		}
	}

	// Strictly after a cursor: feeding row[1]'s cursor returns rows[2:].
	after := data.ActivityCursor{CreatedAt: all[1].CreatedAt, ID: all[1].ID}
	rest, err := data.ActivitySince(ctx, db, []string{p1.ID, p2.ID}, after, 50)
	if err != nil {
		t.Fatalf("ActivitySince after: %v", err)
	}
	if len(rest) != 4 {
		t.Fatalf("want 4 rows after cursor, got %d", len(rest))
	}
	if rest[0].ID != all[2].ID {
		t.Fatalf("cursor not exclusive: got %s, want %s", rest[0].ID, all[2].ID)
	}

	// Bounded by the clamped limit, still oldest-first.
	page, err := data.ActivitySince(ctx, db, []string{p1.ID, p2.ID}, zero, 2)
	if err != nil {
		t.Fatalf("ActivitySince limit: %v", err)
	}
	if len(page) != 2 || page[0].ID != all[0].ID || page[1].ID != all[1].ID {
		t.Fatalf("limit/order wrong: %+v", page)
	}

	// Scoped to the project set — P2's two events are excluded.
	only1, err := data.ActivitySince(ctx, db, []string{p1.ID}, zero, 50)
	if err != nil {
		t.Fatalf("ActivitySince scoped: %v", err)
	}
	if len(only1) != 4 {
		t.Fatalf("want 4 P1 rows, got %d", len(only1))
	}
	for _, a := range only1 {
		if a.ProjectID != p1.ID {
			t.Fatalf("leaked a P2 row: %s", a.ProjectID)
		}
	}

	// No accessible projects → empty, never all-rows.
	none, err := data.ActivitySince(ctx, db, nil, zero, 50)
	if err != nil {
		t.Fatalf("ActivitySince empty: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("want 0 rows for empty project set, got %d", len(none))
	}
}
