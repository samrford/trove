package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"strings"
	"time"

	"trove/backend/internal/data"
	"trove/backend/internal/events"
)

// catchUpCap bounds a single reconnect replay. If a client was offline long
// enough to hit it, we replay this many then emit `resync` so it does a clean
// full reload rather than resume from a known-incomplete position.
const catchUpCap = 200

type EventsHandler struct {
	db  *sql.DB
	hub *events.Hub
}

func NewEventsHandler(db *sql.DB, hub *events.Hub) *EventsHandler {
	return &EventsHandler{db: db, hub: hub}
}

// HandleStream serves GET /v1/events — one long-lived SSE stream per user,
// authenticated by the existing AuthMiddleware (fetch-event-source sends the
// Bearer header, so no SSE-specific auth path is needed). Optional
// ?since=&since_id= (or the Last-Event-ID header on reconnect) replays
// everything newer across the user's accessible projects before going live, so
// a reconnect is lossless.
func (h *EventsHandler) HandleStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID := GetUserID(r.Context())

	projects, err := data.ListProjectsForUser(r.Context(), h.db, userID)
	if err != nil {
		log.Printf("sse list projects: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}
	projectIDs := make([]string, len(projects))
	for i, p := range projects {
		projectIDs[i] = p.ID
	}

	// Catch-up cursor: explicit query params, else the Last-Event-ID header
	// (fetch-event-source resends it on reconnect).
	var cursor *data.ActivityCursor
	var cursorBad bool
	q := r.URL.Query()
	raw := ""
	if s := q.Get("since"); s != "" {
		raw = s + "|" + q.Get("since_id")
	} else if le := r.Header.Get("Last-Event-ID"); le != "" {
		raw = le
	}
	if raw != "" {
		if c, ok := events.ParseCursor(raw); ok {
			cursor = &c
		} else {
			cursorBad = true
			log.Printf("sse: malformed cursor %q", raw)
		}
	}

	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Defeat proxy response buffering (nginx / Fly edge) — without this the
	// stream can be withheld until the connection closes.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(": connected\n\n")); err != nil {
		return
	}
	if rc.Flush() != nil {
		return
	}

	// Subscribe BEFORE catch-up so an event landing during the replay window
	// is queued rather than lost. Cross-boundary duplicates are harmless — the
	// client reducer is idempotent by cursor / item id.
	conn := h.hub.Subscribe(userID, projectIDs)
	defer h.hub.Unsubscribe(conn)

	if cursorBad {
		// Couldn't parse what the client sent — tell it to reload cleanly
		// rather than silently skipping the gap.
		writeEvent(w, rc, events.Message{Event: events.EventResync})
	} else if cursor != nil {
		missed, err := data.ActivitySince(r.Context(), h.db, projectIDs, *cursor, catchUpCap)
		if err != nil {
			// Catch-up failed → don't silently resume with a gap; tell the
			// client to reload cleanly. Live events keep flowing after.
			log.Printf("sse catch-up: %v", err)
			writeEvent(w, rc, events.Message{Event: events.EventResync})
		} else {
			for i := range missed {
				msgs, err := events.Build(r.Context(), h.db, &missed[i])
				if err != nil {
					log.Printf("sse catch-up build: %v", err)
					continue
				}
				for _, m := range msgs {
					if !writeEvent(w, rc, m) {
						return
					}
				}
			}
			// Hit the cap → there may be more between here and live; tell the
			// client to reload cleanly instead of resuming with a gap. `>=`
			// rather than `==` so this survives a future maxActivityLimit drop.
			if len(missed) >= catchUpCap {
				writeEvent(w, rc, events.Message{Event: events.EventResync})
			}
		}
	}

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done(): // client disconnected
			return
		case <-conn.Done(): // hub shutting down / evicted this conn
			return
		case <-heartbeat.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			if rc.Flush() != nil {
				return
			}
		case m := <-conn.C():
			if !writeEvent(w, rc, m) {
				return
			}
		}
	}
}

// writeEvent renders one SSE frame and flushes. Returns false if the socket is
// gone (caller should return). JSON is compact (no embedded newlines), so a
// single data: line is always valid.
func writeEvent(w http.ResponseWriter, rc *http.ResponseController, m events.Message) bool {
	var b strings.Builder
	if m.Cursor != "" {
		b.WriteString("id: ")
		b.WriteString(m.Cursor)
		b.WriteByte('\n')
	}
	b.WriteString("event: ")
	b.WriteString(m.Event)
	b.WriteString("\ndata: ")
	if len(m.Data) > 0 {
		b.Write(m.Data)
	} else {
		b.WriteString("{}")
	}
	b.WriteString("\n\n")
	if _, err := w.Write([]byte(b.String())); err != nil {
		return false
	}
	return rc.Flush() == nil
}
