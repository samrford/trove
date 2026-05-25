package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sergi/go-diff/diffmatchpatch"

	"trove/backend/internal/data"
)

// --- Event payload helpers ---

func itemSnapshot(i *data.Item) map[string]any {
	return map[string]any{"id": i.ID, "seq": i.Sequence, "title": i.Title, "kind": i.Kind}
}

func projectSnapshot(p *data.Project) map[string]any {
	return map[string]any{"slug": p.Slug, "name": p.Name}
}

func diffPair(old, new any) map[string]any {
	return map[string]any{"old": old, "new": new}
}

func eqStrPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func strOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// maxTextDiffBytes bounds the inputs we'll diff.
const maxTextDiffBytes = 16 << 10

// textChange records the *delta* of a long-text field as a diff-match-patch
// patch. Inputs are capped so a row can't bloat unbounded.
func textChange(old, new *string) map[string]any {
	o, n := strOrEmpty(old), strOrEmpty(new)
	if len(o) > maxTextDiffBytes || len(n) > maxTextDiffBytes {
		return map[string]any{
			"truncated": true,
			"old_lines": strings.Count(o, "\n") + 1,
			"new_lines": strings.Count(n, "\n") + 1,
		}
	}
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffCleanupSemantic(dmp.DiffMain(o, n, false))
	return map[string]any{"diff": dmp.PatchToText(dmp.PatchMake(o, diffs))}
}

// itemDiff returns only the fields that actually changed. `body` records just
// {changed:true}: it's long-form markdown not worth storing per edit (the
// current body lives on the item; the feed only renders "edited the notes").
// Everything else keeps full old/new — the AI gets the real delta.
func itemDiff(before, after *data.Item) map[string]any {
	d := map[string]any{}
	if before.Title != after.Title {
		d["title"] = diffPair(before.Title, after.Title)
	}
	if before.Kind != after.Kind {
		d["kind"] = diffPair(before.Kind, after.Kind)
	}
	if before.Status != after.Status {
		d["status"] = diffPair(before.Status, after.Status)
	}
	if before.Position != after.Position {
		d["position"] = diffPair(before.Position, after.Position)
	}
	if !eqStrPtr(before.Body, after.Body) {
		d["body"] = textChange(before.Body, after.Body)
	}
	return d
}

func projectDiff(before, after *data.Project) map[string]any {
	d := map[string]any{}
	if before.Name != after.Name {
		d["name"] = diffPair(before.Name, after.Name)
	}
	if before.Slug != after.Slug {
		d["slug"] = diffPair(before.Slug, after.Slug)
	}
	if !eqStrPtr(before.Description, after.Description) {
		d["description"] = textChange(before.Description, after.Description)
	}
	if !eqStrPtr(before.Colour, after.Colour) {
		d["colour"] = diffPair(before.Colour, after.Colour)
	}
	if !eqStrPtr(before.Icon, after.Icon) {
		d["icon"] = diffPair(before.Icon, after.Icon)
	}
	return d
}

// --- GET endpoint ---

type ActivityHandler struct {
	db *sql.DB
}

func NewActivityHandler(db *sql.DB) *ActivityHandler {
	return &ActivityHandler{db: db}
}

// HandleForProject serves GET /v1/projects/{slug}/activity. Owner/member-only
// (scoped by GetProjectForUser). Keyset-paginated; the response's `next`
// cursor is fed back as ?before=&before_id= — the same shape SSE catch-up will
// reuse. Filters: repeatable `action`, `item` (UUID), `actor` (UUID).
func (h *ActivityHandler) HandleForProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	slugOrID := r.PathValue("slug")
	if slugOrID == "" {
		http.Error(w, `{"error":"Project identifier required"}`, http.StatusBadRequest)
		return
	}

	userID := GetUserID(r.Context())
	project, err := data.GetProjectForUser(r.Context(), h.db, userID, slugOrID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, `{"error":"Project not found"}`, http.StatusNotFound)
			return
		}
		log.Printf("GetProjectForUser: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}

	q := r.URL.Query()
	filter := data.ActivityFilter{ProjectID: project.ID}

	for _, a := range q["action"] {
		if a = strings.TrimSpace(a); a != "" {
			filter.Actions = append(filter.Actions, data.ActivityAction(a))
		}
	}
	if it := strings.TrimSpace(q.Get("item")); it != "" {
		filter.ItemID = &it
	}
	if ac := strings.TrimSpace(q.Get("actor")); ac != "" {
		filter.ActorID = &ac
	}
	if l := strings.TrimSpace(q.Get("limit")); l != "" {
		if n, e := strconv.Atoi(l); e == nil {
			filter.Limit = n
		}
	}

	before := strings.TrimSpace(q.Get("before"))
	beforeID := strings.TrimSpace(q.Get("before_id"))
	if before != "" || beforeID != "" {
		ts, e := time.Parse(time.RFC3339Nano, before)
		if e != nil || beforeID == "" {
			http.Error(w, `{"error":"Invalid cursor — pass before (RFC3339Nano) and before_id together"}`, http.StatusBadRequest)
			return
		}
		filter.Before = &data.ActivityCursor{CreatedAt: ts, ID: beforeID}
	}

	rows, err := data.ListActivity(r.Context(), h.db, filter)
	if err != nil {
		log.Printf("ListActivity: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}

	resp := map[string]any{"activity": rows, "next": nil}
	// A full page implies there may be more — hand back a cursor. (Worst case:
	// one extra request that returns []. Standard keyset tradeoff.)
	if page := data.ClampActivityLimit(filter.Limit); len(rows) == page {
		last := rows[len(rows)-1]
		resp["next"] = map[string]any{
			"before":    last.CreatedAt.Format(time.RFC3339Nano),
			"before_id": last.ID,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("encode activity: %v", err)
	}
}
