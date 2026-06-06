package events_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/lib/pq"

	"trove/backend/internal/data"
	"trove/backend/internal/data/datatest"
	"trove/backend/internal/events"
)

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

// waitNotify blocks for a real notification, skipping the nil pq delivers on
// (re)connect. Returns nil on timeout.
func waitNotify(l *pq.Listener, d time.Duration) *pq.Notification {
	deadline := time.After(d)
	for {
		select {
		case n := <-l.Notify:
			if n != nil {
				return n
			}
		case <-deadline:
			return nil
		}
	}
}

func newListener(t *testing.T) *pq.Listener {
	t.Helper()
	l := pq.NewListener(datatest.DSN(t), time.Second, 10*time.Second, nil)
	t.Cleanup(func() { _ = l.Close() })
	if err := l.Listen("trove_events"); err != nil {
		t.Fatalf("listen: %v", err)
	}
	return l
}

// The AFTER INSERT trigger emits a slim envelope on COMMIT.
func TestTriggerNotifiesOnCommit(t *testing.T) {
	db := datatest.OpenTestDB(t)
	l := newListener(t)
	user := datatest.SeedUser(t, db)
	p := datatest.SeedProject(t, db, user)

	a := logActivity(t, db, data.ActivityInput{
		ProjectID: p.ID, ActorID: user, Action: data.ActivityItemCreated,
	})

	n := waitNotify(l, 3*time.Second)
	if n == nil {
		t.Fatal("no notification within 3s")
	}
	var env events.Envelope
	if err := json.Unmarshal([]byte(n.Extra), &env); err != nil {
		t.Fatalf("unmarshal envelope %q: %v", n.Extra, err)
	}
	if env.ActivityID != a.ID || env.ProjectID != p.ID || env.Action != "item.created" {
		t.Fatalf("envelope mismatch: %+v (want activity %s, project %s)", env, a.ID, p.ID)
	}
}

// A rolled-back tx must NOT notify — Postgres only delivers NOTIFY on commit,
// so the no-op-PATCH / failed-mutation cases are correct for free.
func TestNoNotifyOnRollback(t *testing.T) {
	db := datatest.OpenTestDB(t)
	l := newListener(t)
	user := datatest.SeedUser(t, db)
	p := datatest.SeedProject(t, db, user)

	boom := errors.New("boom")
	err := data.WithRetry(context.Background(), db, func(tx *sql.Tx) error {
		if _, e := data.LogActivity(context.Background(), tx, data.ActivityInput{
			ProjectID: p.ID, ActorID: user, Action: data.ActivityItemCreated,
		}); e != nil {
			return e
		}
		return boom // forces rollback after the INSERT
	})
	if !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
	if n := waitNotify(l, time.Second); n != nil {
		t.Fatalf("rolled-back tx still notified: %q", n.Extra)
	}
}

// End-to-end: trigger → NOTIFY → hub → fan-out, filtered by the connection's
// accessible-project set, and connections close on shutdown.
func TestHubFanOutProjectFilter(t *testing.T) {
	db := datatest.OpenTestDB(t)
	dsn := datatest.DSN(t)
	user := datatest.SeedUser(t, db)
	p1 := datatest.SeedProject(t, db, user)
	p2 := datatest.SeedProject(t, db, user)
	item := datatest.SeedItem(t, db, p1.ID, user)

	ctx, cancel := context.WithCancel(context.Background())
	hub := events.NewHub(db, dsn, nil)
	go hub.Run(ctx)
	t.Cleanup(cancel)
	time.Sleep(300 * time.Millisecond) // let the hub's LISTEN establish

	connA := hub.Subscribe(user, []string{p1.ID}) // sees P1
	connB := hub.Subscribe(user, []string{p2.ID}) // sees P2 only

	logActivity(t, db, data.ActivityInput{
		ProjectID: p1.ID, ItemID: &item.ID, ActorID: user,
		Action: data.ActivityItemCreated,
		Payload: map[string]any{"item": map[string]any{
			"id": item.ID, "seq": item.Sequence, "title": item.Title, "kind": "task",
		}},
	})

	// A receives the P1 event (activity.added, plus item.changed since the
	// payload carries an item id).
	gotActivity, gotItem := false, false
	deadline := time.After(3 * time.Second)
	for !(gotActivity && gotItem) {
		select {
		case m := <-connA.C():
			switch m.Event {
			case events.EventActivityAdded:
				gotActivity = true
			case events.EventItemChanged:
				gotItem = true
			}
		case <-deadline:
			t.Fatalf("connA missing events (activity=%v item=%v)", gotActivity, gotItem)
		}
	}

	// B must see nothing — it's scoped to P2.
	select {
	case m := <-connB.C():
		t.Fatalf("connB leaked a P1 event: %s", m.Event)
	case <-time.After(500 * time.Millisecond):
	}

	// Shutdown closes every connection so SSE handlers return.
	cancel()
	select {
	case <-connA.Done():
	case <-time.After(time.Second):
		t.Fatal("connA not closed on hub shutdown")
	}
}

// A client that never drains its send chan should be evicted once the buffer
// (cap 64) fills — the reconnect+catch-up loop is how slow clients recover.
func TestHubDropsSlowClient(t *testing.T) {
	db := datatest.OpenTestDB(t)
	dsn := datatest.DSN(t)
	user := datatest.SeedUser(t, db)
	p := datatest.SeedProject(t, db, user)

	ctx, cancel := context.WithCancel(context.Background())
	hub := events.NewHub(db, dsn, nil)
	go hub.Run(ctx)
	t.Cleanup(cancel)
	time.Sleep(300 * time.Millisecond) // let the hub's LISTEN establish

	conn := hub.Subscribe(user, []string{p.ID})

	// > 64 messages with no consumer → buffer fills → next fanOut Unsubscribes
	// (each activity here emits only activity.added — empty payload, no item).
	for i := 0; i < 80; i++ {
		logActivity(t, db, data.ActivityInput{
			ProjectID: p.ID, ActorID: user, Action: data.ActivityItemCreated,
		})
	}

	select {
	case <-conn.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("slow client not dropped within 5s")
	}
}

// project.created / project.updated → activity.added + project.changed
// (Build pulls the full row via GetProjectByID).
func TestHubProjectChanged(t *testing.T) {
	db := datatest.OpenTestDB(t)
	dsn := datatest.DSN(t)
	user := datatest.SeedUser(t, db)
	p := datatest.SeedProject(t, db, user)

	ctx, cancel := context.WithCancel(context.Background())
	hub := events.NewHub(db, dsn, nil)
	go hub.Run(ctx)
	t.Cleanup(cancel)
	time.Sleep(300 * time.Millisecond)

	conn := hub.Subscribe(user, []string{p.ID})

	logActivity(t, db, data.ActivityInput{
		ProjectID: p.ID, ActorID: user, Action: data.ActivityProjectUpdated,
		Payload: map[string]any{"project": map[string]any{
			"id": p.ID, "slug": p.Slug, "name": p.Name,
		}},
	})

	gotActivity, gotProject := false, false
	deadline := time.After(3 * time.Second)
	for !(gotActivity && gotProject) {
		select {
		case m := <-conn.C():
			switch m.Event {
			case events.EventActivityAdded:
				gotActivity = true
			case events.EventProjectChanged:
				gotProject = true
			}
		case <-deadline:
			t.Fatalf("missing events (activity=%v project=%v)", gotActivity, gotProject)
		}
	}
}

// item.changed payloads MUST be wrapped by the configured ItemHydrator so SSE
// and REST item JSON have the same shape. Without one, bare data.Item lacks
// the `attachments` field the front-end's Item type expects
func TestHubItemChangedHydrator(t *testing.T) {
	t.Run("nil hydrator → ships bare data.Item (no attachments field)", func(t *testing.T) {
		db := datatest.OpenTestDB(t)
		dsn := datatest.DSN(t)
		user := datatest.SeedUser(t, db)
		p := datatest.SeedProject(t, db, user)
		item := datatest.SeedItem(t, db, p.ID, user)

		ctx, cancel := context.WithCancel(context.Background())
		hub := events.NewHub(db, dsn, nil)
		go hub.Run(ctx)
		t.Cleanup(cancel)
		time.Sleep(300 * time.Millisecond)
		conn := hub.Subscribe(user, []string{p.ID})

		logActivity(t, db, data.ActivityInput{
			ProjectID: p.ID, ItemID: &item.ID, ActorID: user,
			Action: data.ActivityItemUpdated,
			Payload: map[string]any{"item": map[string]any{
				"id": item.ID, "seq": item.Sequence, "title": item.Title, "kind": "task",
			}},
		})

		payload := waitForItemChanged(t, conn, 3*time.Second)
		if _, ok := payload["attachments"]; ok {
			t.Fatalf("bare data.Item should not have attachments — hydrator missing test guard? got: %v", payload)
		}
	})

	t.Run("with hydrator → its output is what ships", func(t *testing.T) {
		db := datatest.OpenTestDB(t)
		dsn := datatest.DSN(t)
		user := datatest.SeedUser(t, db)
		p := datatest.SeedProject(t, db, user)
		item := datatest.SeedItem(t, db, p.ID, user)

		hydrator := func(_ context.Context, it *data.Item) (any, error) {
			// Production hydrator wraps with tags + signed attachments; here we
			// just attach a marker and the missing `attachments` field so the
			// test asserts the wrap was applied, not the wrap's own semantics.
			return map[string]any{
				"id":          it.ID,
				"title":       it.Title,
				"attachments": []any{},
				"tags":        []any{},
				"_hydrated":   true,
			}, nil
		}

		ctx, cancel := context.WithCancel(context.Background())
		hub := events.NewHub(db, dsn, hydrator)
		go hub.Run(ctx)
		t.Cleanup(cancel)
		time.Sleep(300 * time.Millisecond)
		conn := hub.Subscribe(user, []string{p.ID})

		logActivity(t, db, data.ActivityInput{
			ProjectID: p.ID, ItemID: &item.ID, ActorID: user,
			Action: data.ActivityItemUpdated,
			Payload: map[string]any{"item": map[string]any{
				"id": item.ID, "seq": item.Sequence, "title": item.Title, "kind": "task",
			}},
		})

		payload := waitForItemChanged(t, conn, 3*time.Second)
		if payload["_hydrated"] != true {
			t.Fatalf("hydrator output did not reach the wire: %v", payload)
		}
		if _, ok := payload["attachments"]; !ok {
			t.Fatalf("hydrated payload missing attachments: %v", payload)
		}
	})
}

func waitForItemChanged(t *testing.T, conn *events.Conn, d time.Duration) map[string]any {
	t.Helper()
	deadline := time.After(d)
	for {
		select {
		case m := <-conn.C():
			if m.Event != events.EventItemChanged {
				continue
			}
			var payload map[string]any
			if err := json.Unmarshal(m.Data, &payload); err != nil {
				t.Fatalf("unmarshal item.changed: %v", err)
			}
			return payload
		case <-deadline:
			t.Fatal("never got item.changed")
		}
	}
}
