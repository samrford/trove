package handlers

// Stage 2: every mutation handler emits the right activity row (action, actor,
// self-contained payload), and the GET endpoint enforces owner-only access +
// filters + keyset pagination. datatest-backed (real Postgres); the Seed*
// fixtures go through the data layer, not handlers, so they emit no activity —
// a clean baseline.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	photopicker "github.com/samrford/google-photos-picker"

	"trove/backend/internal/data"
	"trove/backend/internal/data/datatest"
	"trove/backend/internal/data/storage/storagetest"
)

func authReq(method, target string, body any) *http.Request {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, target, r)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func asUser(req *http.Request, userID string) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), userIDKey, userID))
}

func activityRows(t *testing.T, db *sql.DB, projectID string) []data.Activity {
	t.Helper()
	rows, err := data.ListActivity(context.Background(), db, data.ActivityFilter{ProjectID: projectID, Limit: 200})
	if err != nil {
		t.Fatalf("ListActivity: %v", err)
	}
	return rows
}

func onlyRow(t *testing.T, db *sql.DB, projectID string) data.Activity {
	t.Helper()
	rows := activityRows(t, db, projectID)
	if len(rows) != 1 {
		t.Fatalf("activity rows = %d, want exactly 1", len(rows))
	}
	return rows[0]
}

func payload(t *testing.T, a data.Activity) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(a.Payload, &m); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	return m
}

func TestActivity_ItemCreate(t *testing.T) {
	db := datatest.OpenTestDB(t)
	user := datatest.SeedUser(t, db)
	project := datatest.SeedProject(t, db, user)
	h := NewItemsHandler(db, nil)

	req := asUser(authReq(http.MethodPost, "/v1/projects/"+project.Slug+"/items",
		map[string]any{"kind": "task", "title": "Ship it"}), user)
	req.SetPathValue("slug", project.Slug)
	w := httptest.NewRecorder()
	h.HandleCollection(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}

	a := onlyRow(t, db, project.ID)
	if a.Action != data.ActivityItemCreated {
		t.Errorf("action = %q, want item.created", a.Action)
	}
	if a.ActorID != user {
		t.Errorf("actor = %q, want %q", a.ActorID, user)
	}
	if a.ItemID == nil {
		t.Error("item_id nil, want set")
	}
	if it := payload(t, a)["item"].(map[string]any); it["title"] != "Ship it" {
		t.Errorf("payload item.title = %v, want 'Ship it'", it["title"])
	}
}

func TestActivity_ItemUpdate(t *testing.T) {
	db := datatest.OpenTestDB(t)
	user := datatest.SeedUser(t, db)
	project := datatest.SeedProject(t, db, user)
	item := datatest.SeedItem(t, db, project.ID, user)
	h := NewItemsHandler(db, nil)

	patch := func(t *testing.T, bodyJSON map[string]any) {
		t.Helper()
		req := asUser(authReq(http.MethodPatch,
			"/v1/projects/"+project.Slug+"/items/"+strconv.Itoa(item.Sequence), bodyJSON), user)
		req.SetPathValue("slug", project.Slug)
		req.SetPathValue("seq", strconv.Itoa(item.Sequence))
		w := httptest.NewRecorder()
		h.HandleByID(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	}

	t.Run("multi-field → one row, diffed", func(t *testing.T) {
		patch(t, map[string]any{"title": "Renamed", "status": "in_progress"})
		a := onlyRow(t, db, project.ID)
		if a.Action != data.ActivityItemUpdated {
			t.Fatalf("action = %q, want item.updated", a.Action)
		}
		diff := payload(t, a)["diff"].(map[string]any)
		if _, ok := diff["title"]; !ok {
			t.Error("diff missing title")
		}
		if _, ok := diff["status"]; !ok {
			t.Error("diff missing status")
		}
		if _, ok := diff["body"]; ok {
			t.Error("diff has body — nothing body-related changed")
		}
		st := diff["status"].(map[string]any)
		if st["old"] != "open" || st["new"] != "in_progress" {
			t.Errorf("status diff = %v, want open→in_progress", st)
		}
	})

	t.Run("no-op PATCH → no new row", func(t *testing.T) {
		before := len(activityRows(t, db, project.ID))
		patch(t, map[string]any{"title": "Renamed"}) // already "Renamed"
		if got := len(activityRows(t, db, project.ID)); got != before {
			t.Errorf("rows = %d, want unchanged %d (no-op must log nothing)", got, before)
		}
	})

	t.Run("position-only is still captured", func(t *testing.T) {
		before := len(activityRows(t, db, project.ID))
		patch(t, map[string]any{"position": 4242})
		rows := activityRows(t, db, project.ID)
		if len(rows) != before+1 {
			t.Fatalf("rows = %d, want %d", len(rows), before+1)
		}
		diff := payload(t, rows[0])["diff"].(map[string]any) // newest first
		if _, ok := diff["position"]; !ok {
			t.Errorf("diff missing position: %v", diff)
		}
	})
}

func TestActivity_ItemDelete_KeepsRowSetsItemNull(t *testing.T) {
	db := datatest.OpenTestDB(t)
	user := datatest.SeedUser(t, db)
	project := datatest.SeedProject(t, db, user)
	item := datatest.SeedItem(t, db, project.ID, user)
	h := NewItemsHandler(db, nil)

	req := asUser(authReq(http.MethodDelete,
		"/v1/projects/"+project.Slug+"/items/"+strconv.Itoa(item.Sequence), nil), user)
	req.SetPathValue("slug", project.Slug)
	req.SetPathValue("seq", strconv.Itoa(item.Sequence))
	w := httptest.NewRecorder()
	h.HandleByID(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", w.Code, w.Body.String())
	}

	a := onlyRow(t, db, project.ID)
	if a.Action != data.ActivityItemDeleted {
		t.Errorf("action = %q, want item.deleted", a.Action)
	}
	if a.ItemID != nil {
		t.Errorf("item_id = %v, want NULL (ON DELETE SET NULL in the same tx)", *a.ItemID)
	}
	if it := payload(t, a)["item"].(map[string]any); it["title"] != item.Title {
		t.Errorf("snapshot title = %v, want %q", it["title"], item.Title)
	}
}

func TestActivity_TagAttachDetach(t *testing.T) {
	db := datatest.OpenTestDB(t)
	user := datatest.SeedUser(t, db)
	project := datatest.SeedProject(t, db, user)
	item := datatest.SeedItem(t, db, project.ID, user)
	h := NewItemsHandler(db, nil)
	base := "/v1/projects/" + project.Slug + "/items/" + strconv.Itoa(item.Sequence) + "/tags"

	add := asUser(authReq(http.MethodPost, base, map[string]any{"name": "urgent"}), user)
	add.SetPathValue("slug", project.Slug)
	add.SetPathValue("seq", strconv.Itoa(item.Sequence))
	wa := httptest.NewRecorder()
	h.HandleItemTags(wa, add)
	if wa.Code != http.StatusCreated {
		t.Fatalf("attach status = %d; body=%s", wa.Code, wa.Body.String())
	}

	del := asUser(authReq(http.MethodDelete, base+"/urgent", nil), user)
	del.SetPathValue("slug", project.Slug)
	del.SetPathValue("seq", strconv.Itoa(item.Sequence))
	del.SetPathValue("tagSlug", "urgent")
	wd := httptest.NewRecorder()
	h.HandleItemTagByID(wd, del)
	if wd.Code != http.StatusNoContent {
		t.Fatalf("detach status = %d; body=%s", wd.Code, wd.Body.String())
	}

	rows := activityRows(t, db, project.ID) // newest first
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0].Action != data.ActivityItemTagRemoved || rows[1].Action != data.ActivityItemTagAdded {
		t.Fatalf("actions = [%s, %s], want [item.tag_removed, item.tag_added]", rows[0].Action, rows[1].Action)
	}
	if tag := payload(t, rows[1])["tag"].(map[string]any); tag["name"] != "urgent" {
		t.Errorf("tag_added payload tag.name = %v, want 'urgent'", tag["name"])
	}
}

func TestActivity_ProjectLifecycle(t *testing.T) {
	db := datatest.OpenTestDB(t)
	user := datatest.SeedUser(t, db)
	h := NewProjectsHandler(db)

	// create
	cw := httptest.NewRecorder()
	h.HandleCollection(cw, asUser(authReq(http.MethodPost, "/v1/projects",
		map[string]any{"name": "Garden", "slug": "garden"}), user))
	if cw.Code != http.StatusCreated {
		t.Fatalf("create status = %d; body=%s", cw.Code, cw.Body.String())
	}
	var created data.Project
	if err := json.Unmarshal(cw.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created project: %v", err)
	}

	// update (projects.HandleByID parses slug from the URL path)
	uw := httptest.NewRecorder()
	h.HandleByID(uw, asUser(authReq(http.MethodPatch, "/v1/projects/garden",
		map[string]any{"name": "Garden Plans", "slug": "garden"}), user))
	if uw.Code != http.StatusOK {
		t.Fatalf("update status = %d; body=%s", uw.Code, uw.Body.String())
	}

	rows := activityRows(t, db, created.ID)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (created + updated)", len(rows))
	}
	if rows[0].Action != data.ActivityProjectUpdated || rows[1].Action != data.ActivityProjectCreated {
		t.Fatalf("actions = [%s, %s], want [project.updated, project.created]", rows[0].Action, rows[1].Action)
	}
	if d := payload(t, rows[0])["diff"].(map[string]any); d["name"] == nil {
		t.Errorf("project.updated diff missing name: %v", d)
	}

	// delete → cascade wipes ALL of this project's activity (incl. the two above)
	dw := httptest.NewRecorder()
	h.HandleByID(dw, asUser(authReq(http.MethodDelete, "/v1/projects/garden", nil), user))
	if dw.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d; body=%s", dw.Code, dw.Body.String())
	}
	if n := len(activityRows(t, db, created.ID)); n != 0 {
		t.Errorf("activity after project delete = %d, want 0 (ON DELETE CASCADE; delete logs nothing)", n)
	}
}

func TestActivity_Endpoint(t *testing.T) {
	db := datatest.OpenTestDB(t)
	owner := datatest.SeedUser(t, db)
	project := datatest.SeedProject(t, db, owner)
	items := NewItemsHandler(db, nil)

	// 3 item.created rows via the handler.
	for i := 0; i < 3; i++ {
		req := asUser(authReq(http.MethodPost, "/v1/projects/"+project.Slug+"/items",
			map[string]any{"kind": "task", "title": "i" + strconv.Itoa(i)}), owner)
		req.SetPathValue("slug", project.Slug)
		w := httptest.NewRecorder()
		items.HandleCollection(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("seed item %d: status %d", i, w.Code)
		}
	}

	ah := NewActivityHandler(db)
	get := func(t *testing.T, userID, query string) *httptest.ResponseRecorder {
		t.Helper()
		req := asUser(authReq(http.MethodGet, "/v1/projects/"+project.Slug+"/activity"+query, nil), userID)
		req.SetPathValue("slug", project.Slug)
		w := httptest.NewRecorder()
		ah.HandleForProject(w, req)
		return w
	}

	t.Run("stranger gets 404", func(t *testing.T) {
		stranger := datatest.SeedUser(t, db)
		if w := get(t, stranger, ""); w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (no access)", w.Code)
		}
	})

	t.Run("owner sees all, next is null", func(t *testing.T) {
		w := get(t, owner, "")
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
		}
		var resp struct {
			Activity []data.Activity `json:"activity"`
			Next     *struct {
				Before   string `json:"before"`
				BeforeID string `json:"before_id"`
			} `json:"next"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(resp.Activity) != 3 {
			t.Errorf("activity len = %d, want 3", len(resp.Activity))
		}
		if resp.Next != nil {
			t.Errorf("next = %+v, want null (3 < page size)", resp.Next)
		}
	})

	t.Run("action filter", func(t *testing.T) {
		w := get(t, owner, "?action=item.created")
		var resp struct {
			Activity []data.Activity `json:"activity"`
		}
		json.Unmarshal(w.Body.Bytes(), &resp)
		if len(resp.Activity) != 3 {
			t.Errorf("len = %d, want 3", len(resp.Activity))
		}
		w2 := get(t, owner, "?action=project.created")
		var resp2 struct {
			Activity []data.Activity `json:"activity"`
		}
		json.Unmarshal(w2.Body.Bytes(), &resp2)
		if len(resp2.Activity) != 0 {
			t.Errorf("len = %d, want 0 (no project.created here)", len(resp2.Activity))
		}
	})

	t.Run("cursor pagination", func(t *testing.T) {
		w := get(t, owner, "?limit=2")
		var p1 struct {
			Activity []data.Activity `json:"activity"`
			Next     *struct {
				Before   string `json:"before"`
				BeforeID string `json:"before_id"`
			} `json:"next"`
		}
		json.Unmarshal(w.Body.Bytes(), &p1)
		if len(p1.Activity) != 2 || p1.Next == nil {
			t.Fatalf("page1 len=%d next=%v, want 2 + cursor", len(p1.Activity), p1.Next)
		}
		w2 := get(t, owner, "?limit=2&before="+p1.Next.Before+"&before_id="+p1.Next.BeforeID)
		var p2 struct {
			Activity []data.Activity `json:"activity"`
		}
		json.Unmarshal(w2.Body.Bytes(), &p2)
		if len(p2.Activity) != 1 {
			t.Fatalf("page2 len = %d, want 1", len(p2.Activity))
		}
		if p2.Activity[0].ID == p1.Activity[0].ID || p2.Activity[0].ID == p1.Activity[1].ID {
			t.Error("page2 overlaps page1")
		}
	})
}

// TestActivity_GPSink covers the best-effort seam: the storage upload is
// outside the tx, but the attachment row + its activity row commit together.
func TestActivity_GPSink(t *testing.T) {
	db := datatest.OpenTestDB(t)
	user := datatest.SeedUser(t, db)
	project := datatest.SeedProject(t, db, user)
	item := datatest.SeedItem(t, db, project.ID, user)

	sink := NewTroveSink(db, storagetest.NewFake())
	id, err := sink.SavePhoto(context.Background(), user, "job-1", photopicker.DownloadedPhoto{
		Filename: "vacation.jpg",
		MimeType: "image/jpeg",
		Bytes:    strings.NewReader("photo-bytes"),
		JobMetadata: map[string]string{
			"project_id": project.ID,
			"item_id":    item.ID,
		},
	})
	if err != nil {
		t.Fatalf("SavePhoto: %v", err)
	}
	if id == "" {
		t.Error("returned attachment id empty")
	}

	a := onlyRow(t, db, project.ID)
	if a.Action != data.ActivityAttachmentAdded {
		t.Errorf("action = %q, want attachment.added", a.Action)
	}
	if a.ActorID != user {
		t.Errorf("actor = %q, want %q (import starter)", a.ActorID, user)
	}
	pm := payload(t, a)
	att := pm["attachment"].(map[string]any)
	if att["source"] != string(data.AttachmentSourceGooglePhotos) {
		t.Errorf("attachment.source = %v, want google_photos", att["source"])
	}
	if it := pm["item"].(map[string]any); it["title"] != item.Title {
		t.Errorf("item snapshot title = %v, want %q (GetItemByID)", it["title"], item.Title)
	}
}
