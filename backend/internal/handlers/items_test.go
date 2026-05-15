package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"

	"trove/backend/internal/data"
)

// kindTaskBody builds a JSON create body referencing the typed enum constant,
// so a renamed enum value breaks these tests rather than silently shipping a
// stale string.
func kindTaskBody(title string) string {
	return fmt.Sprintf(`{"kind":%q,"title":%q}`, string(data.ItemKindTask), title)
}

func itemRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "project_id", "sequence", "kind", "status", "title", "body",
		"position", "creator_id", "created_at", "updated_at",
	}).AddRow(testItemID, testProjectID, 1, string(data.ItemKindTask), string(data.ItemStatusOpen), "T", nil, 1.0, testUserID, time.Now(), time.Now())
}

// tagRows is shared with tags_test.go.
func tagRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "slug", "name", "name_normalised", "description", "icon", "colour",
		"user_id", "group_id", "archived_at", "created_at", "updated_at",
	}).AddRow(testTagID, "bug", "Bug", "bug", nil, nil, nil, testUserID, nil, nil, time.Now(), time.Now())
}

// emptyItemTagRows is the column set returned by GetTagsForItem with no rows.
func emptyItemTagRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "slug", "name", "name_normalised", "description", "icon", "colour",
		"user_id", "group_id", "archived_at", "created_at", "updated_at",
	})
}

// tagsForItemsRows is the GetTagsForItems shape: item_id followed by tag cols.
func tagsForItemsRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"item_id",
		"id", "slug", "name", "name_normalised", "description", "icon", "colour",
		"user_id", "group_id", "archived_at", "created_at", "updated_at",
	}).AddRow(testItemID, testTagID, "bug", "Bug", "bug", nil, nil, nil, testUserID, nil, nil, time.Now(), time.Now())
}

func mockProjectLookup(t *testing.T, mock sqlmock.Sqlmock, ownerID string) {
	t.Helper()
	rows := sqlmock.NewRows([]string{
		"id", "slug", "name", "description", "colour", "icon",
		"owner_id", "archived_at", "created_at", "updated_at",
	}).AddRow(testProjectID, "foo", "Foo", nil, nil, nil, ownerID, nil, time.Now(), time.Now())
	mock.ExpectQuery(`FROM projects p`).
		WithArgs(testUserID, "foo").
		WillReturnRows(rows)
}

// itemCase drives one subtest against an ItemsHandler entrypoint. setup scripts
// the sqlmock expectations; check (optional) asserts the response body once the
// status code has matched want.
type itemCase struct {
	name    string
	method  string
	call    string // collection | byid | itemtags | itemtagbyid
	slug    string
	seq     string
	tagSlug string
	query   string
	body    string
	setup   func(t *testing.T, m sqlmock.Sqlmock)
	want    int
	check   func(t *testing.T, w *httptest.ResponseRecorder)
}

func runItemCases(t *testing.T, cases []itemCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := newMockDB(t)
			if tc.setup != nil {
				tc.setup(t, mock)
			}
			h := NewItemsHandler(db)

			url := "/v1/projects/foo/items"
			if tc.query != "" {
				url += "?" + tc.query
			}
			r := authedReq(tc.method, url, tc.body)
			if tc.slug != "" {
				r.SetPathValue("slug", tc.slug)
			}
			if tc.seq != "" {
				r.SetPathValue("seq", tc.seq)
			}
			if tc.tagSlug != "" {
				r.SetPathValue("tagSlug", tc.tagSlug)
			}
			w := httptest.NewRecorder()

			switch tc.call {
			case "collection":
				h.HandleCollection(w, r)
			case "byid":
				h.HandleByID(w, r)
			case "itemtags":
				h.HandleItemTags(w, r)
			case "itemtagbyid":
				h.HandleItemTagByID(w, r)
			default:
				t.Fatalf("unknown call %q", tc.call)
			}

			if w.Code != tc.want {
				t.Fatalf("status: got %d want %d body=%s", w.Code, tc.want, w.Body.String())
			}
			if tc.check != nil {
				tc.check(t, w)
			}
		})
	}
}

// assertItemID decodes the body as a single item and checks its ID.
func assertItemID(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	var it data.Item
	decodeJSON(t, w, &it)
	if it.ID != testItemID {
		t.Errorf("item id: got %q want %q", it.ID, testItemID)
	}
}

func TestItems_Collection(t *testing.T) {
	runItemCases(t, []itemCase{
		{
			name: "MissingSlug", method: "GET", call: "collection", want: http.StatusBadRequest,
		},
		{
			name: "MethodNotAllowed", method: "PUT", call: "collection", slug: "foo",
			want: http.StatusMethodNotAllowed,
		},
		{
			name: "List_Success", method: "GET", call: "collection", slug: "foo",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items i`).WithArgs(testProjectID).WillReturnRows(itemRows())
				m.ExpectQuery(`WHERE it\.item_id = ANY`).
					WithArgs(pq.Array([]string{testItemID})).
					WillReturnRows(tagsForItemsRows())
			},
			want: http.StatusOK,
			check: func(t *testing.T, w *httptest.ResponseRecorder) {
				var items []data.Item
				decodeJSON(t, w, &items)
				if len(items) != 1 {
					t.Fatalf("want 1 item, got %d", len(items))
				}
				if items[0].ID != testItemID {
					t.Errorf("item id: got %q", items[0].ID)
				}
				// The batched tag fetch must actually hang the tag off the
				// item — previously asserted only via status code.
				if len(items[0].Tags) != 1 || items[0].Tags[0].Slug != "bug" {
					t.Errorf("expected item to carry the 'bug' tag, got %+v", items[0].Tags)
				}
			},
		},
		{
			name: "List_ProjectNotFound", method: "GET", call: "collection", slug: "foo",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM projects p`).WithArgs(testUserID, "foo").WillReturnError(sql.ErrNoRows)
			},
			want: http.StatusNotFound,
		},
		{
			name: "List_ProjectLookupError", method: "GET", call: "collection", slug: "foo",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM projects p`).WithArgs(testUserID, "foo").WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
		{
			name: "List_BadKind", method: "GET", call: "collection", slug: "foo", query: "kind=nope",
			setup: func(t *testing.T, m sqlmock.Sqlmock) { mockProjectLookup(t, m, testUserID) },
			want:  http.StatusBadRequest,
		},
		{
			name: "List_BadStatus", method: "GET", call: "collection", slug: "foo", query: "status=nope",
			setup: func(t *testing.T, m sqlmock.Sqlmock) { mockProjectLookup(t, m, testUserID) },
			want:  http.StatusBadRequest,
		},
		{
			name: "List_BadTagMode", method: "GET", call: "collection", slug: "foo", query: "tag_mode=xor",
			setup: func(t *testing.T, m sqlmock.Sqlmock) { mockProjectLookup(t, m, testUserID) },
			want:  http.StatusBadRequest,
		},
		{
			name: "List_DBError", method: "GET", call: "collection", slug: "foo",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items i`).WithArgs(testProjectID).WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
		{
			name: "List_TagsLookupError", method: "GET", call: "collection", slug: "foo",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items i`).WithArgs(testProjectID).WillReturnRows(itemRows())
				m.ExpectQuery(`WHERE it\.item_id = ANY`).
					WithArgs(pq.Array([]string{testItemID})).
					WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
		{
			name: "Create_Success", method: "POST", call: "collection", slug: "foo", body: kindTaskBody("T"),
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectBegin()
				m.ExpectQuery(`UPDATE project_item_sequences`).
					WithArgs(testProjectID).
					WillReturnRows(sqlmock.NewRows([]string{"seq"}).AddRow(1))
				m.ExpectQuery(`INSERT INTO items`).
					WithArgs(testProjectID, 1, string(data.ItemKindTask), "T", sql.NullString{}, testUserID).
					WillReturnRows(itemRows())
				m.ExpectCommit()
			},
			want:  http.StatusCreated,
			check: assertItemID,
		},
		{
			name: "Create_Forbidden", method: "POST", call: "collection", slug: "foo", body: kindTaskBody("T"),
			setup: func(t *testing.T, m sqlmock.Sqlmock) { mockProjectLookup(t, m, "other-owner") },
			want:  http.StatusForbidden,
		},
		{
			name: "Create_InvalidJSON", method: "POST", call: "collection", slug: "foo", body: `{bad`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) { mockProjectLookup(t, m, testUserID) },
			want:  http.StatusBadRequest,
		},
		{
			name: "Create_MissingTitle", method: "POST", call: "collection", slug: "foo", body: kindTaskBody("   "),
			setup: func(t *testing.T, m sqlmock.Sqlmock) { mockProjectLookup(t, m, testUserID) },
			want:  http.StatusBadRequest,
		},
		{
			name: "Create_BadKind", method: "POST", call: "collection", slug: "foo", body: `{"kind":"x","title":"T"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) { mockProjectLookup(t, m, testUserID) },
			want:  http.StatusBadRequest,
		},
		{
			name: "Create_DBError", method: "POST", call: "collection", slug: "foo", body: kindTaskBody("T"),
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectBegin().WillReturnError(errors.New("nope"))
			},
			want: http.StatusInternalServerError,
		},
	})
}

func TestItems_ByID(t *testing.T) {
	runItemCases(t, []itemCase{
		{
			name: "MissingSeq", method: "GET", call: "byid", slug: "foo",
			want: http.StatusBadRequest,
		},
		{
			name: "BadSeq", method: "GET", call: "byid", slug: "foo", seq: "abc",
			want: http.StatusBadRequest,
		},
		{
			name: "MethodNotAllowed", method: "PUT", call: "byid", slug: "foo", seq: "1",
			want: http.StatusMethodNotAllowed,
		},
		{
			name: "Get_Success", method: "GET", call: "byid", slug: "foo", seq: "1",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id = \$1 AND sequence = \$2`).
					WithArgs(testProjectID, 1).WillReturnRows(itemRows())
				m.ExpectQuery(`FROM tags t\s+JOIN item_tags`).
					WithArgs(testItemID).WillReturnRows(emptyItemTagRows())
			},
			want: http.StatusOK,
			check: func(t *testing.T, w *httptest.ResponseRecorder) {
				var it data.Item
				decodeJSON(t, w, &it)
				if it.ID != testItemID {
					t.Errorf("item id: got %q", it.ID)
				}
				if len(it.Tags) != 0 {
					t.Errorf("expected no tags, got %+v", it.Tags)
				}
			},
		},
		{
			name: "Get_NotFound", method: "GET", call: "byid", slug: "foo", seq: "1",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnError(sql.ErrNoRows)
			},
			want: http.StatusNotFound,
		},
		{
			name: "Get_DBError", method: "GET", call: "byid", slug: "foo", seq: "1",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
		{
			name: "Get_TagsError", method: "GET", call: "byid", slug: "foo", seq: "1",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnRows(itemRows())
				m.ExpectQuery(`FROM tags t\s+JOIN item_tags`).WithArgs(testItemID).WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
		{
			name: "Update_Success", method: "PATCH", call: "byid", slug: "foo", seq: "1", body: `{"title":"Updated"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnRows(itemRows())
				m.ExpectQuery(`UPDATE items SET`).WithArgs("Updated", testItemID).WillReturnRows(itemRows())
				m.ExpectQuery(`FROM tags t\s+JOIN item_tags`).WithArgs(testItemID).WillReturnRows(emptyItemTagRows())
			},
			want:  http.StatusOK,
			check: assertItemID,
		},
		{
			name: "Update_ClearBody", method: "PATCH", call: "byid", slug: "foo", seq: "1", body: `{"body":"   "}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnRows(itemRows())
				m.ExpectQuery(`UPDATE items SET .*body`).WithArgs(sql.NullString{}, testItemID).WillReturnRows(itemRows())
				m.ExpectQuery(`FROM tags t\s+JOIN item_tags`).WithArgs(testItemID).WillReturnRows(emptyItemTagRows())
			},
			want:  http.StatusOK,
			check: assertItemID,
		},
		{
			name: "Update_AllFields", method: "PATCH", call: "byid", slug: "foo", seq: "1",
			body: fmt.Sprintf(`{"title":"New","body":"body","kind":%q,"status":%q,"position":1.5}`,
				string(data.ItemKindTask), string(data.ItemStatusDone)),
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnRows(itemRows())
				m.ExpectQuery(`UPDATE items SET`).
					WithArgs("New", "body", string(data.ItemKindTask), string(data.ItemStatusDone), 1.5, testItemID).
					WillReturnRows(itemRows())
				m.ExpectQuery(`FROM tags t\s+JOIN item_tags`).WithArgs(testItemID).WillReturnRows(emptyItemTagRows())
			},
			want:  http.StatusOK,
			check: assertItemID,
		},
		{
			name: "Update_ItemNotFound", method: "PATCH", call: "byid", slug: "foo", seq: "1", body: `{"title":"x"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnError(sql.ErrNoRows)
			},
			want: http.StatusNotFound,
		},
		{
			name: "Update_ItemLookupError", method: "PATCH", call: "byid", slug: "foo", seq: "1", body: `{"title":"x"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
		{
			name: "Update_InvalidJSON", method: "PATCH", call: "byid", slug: "foo", seq: "1", body: `{bad`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnRows(itemRows())
			},
			want: http.StatusBadRequest,
		},
		{
			name: "Update_EmptyTitle", method: "PATCH", call: "byid", slug: "foo", seq: "1", body: `{"title":"  "}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnRows(itemRows())
			},
			want: http.StatusBadRequest,
		},
		{
			name: "Update_BadKind", method: "PATCH", call: "byid", slug: "foo", seq: "1", body: `{"kind":"nope"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnRows(itemRows())
			},
			want: http.StatusBadRequest,
		},
		{
			name: "Update_BadStatus", method: "PATCH", call: "byid", slug: "foo", seq: "1", body: `{"status":"nope"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnRows(itemRows())
			},
			want: http.StatusBadRequest,
		},
		{
			name: "Update_UpdateError", method: "PATCH", call: "byid", slug: "foo", seq: "1", body: `{"title":"x"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnRows(itemRows())
				m.ExpectQuery(`UPDATE items SET`).WithArgs("x", testItemID).WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
		{
			name: "Update_TagsError", method: "PATCH", call: "byid", slug: "foo", seq: "1", body: `{"title":"x"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnRows(itemRows())
				m.ExpectQuery(`UPDATE items SET`).WithArgs("x", testItemID).WillReturnRows(itemRows())
				m.ExpectQuery(`FROM tags t\s+JOIN item_tags`).WithArgs(testItemID).WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
		{
			name: "Update_Forbidden", method: "PATCH", call: "byid", slug: "foo", seq: "1", body: `{"title":"x"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) { mockProjectLookup(t, m, "other-owner") },
			want:  http.StatusForbidden,
		},
		{
			name: "Delete_Success", method: "DELETE", call: "byid", slug: "foo", seq: "1",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnRows(itemRows())
				m.ExpectExec(`DELETE FROM items`).WithArgs(testItemID).WillReturnResult(sqlmock.NewResult(0, 1))
			},
			want: http.StatusNoContent,
		},
		{
			name: "Delete_NotFound", method: "DELETE", call: "byid", slug: "foo", seq: "1",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnError(sql.ErrNoRows)
			},
			want: http.StatusNotFound,
		},
		{
			name: "Delete_LookupError", method: "DELETE", call: "byid", slug: "foo", seq: "1",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
		{
			name: "Delete_DBError", method: "DELETE", call: "byid", slug: "foo", seq: "1",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnRows(itemRows())
				m.ExpectExec(`DELETE FROM items`).WithArgs(testItemID).WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
		{
			name: "Delete_Forbidden", method: "DELETE", call: "byid", slug: "foo", seq: "1",
			setup: func(t *testing.T, m sqlmock.Sqlmock) { mockProjectLookup(t, m, "other-owner") },
			want:  http.StatusForbidden,
		},
	})
}

func TestItems_AttachTag(t *testing.T) {
	// assertTagID checks the attach response echoes the resolved tag.
	assertTagID := func(t *testing.T, w *httptest.ResponseRecorder) {
		t.Helper()
		var tag data.Tag
		decodeJSON(t, w, &tag)
		if tag.ID != testTagID || tag.Slug != "bug" {
			t.Errorf("tag: got id=%q slug=%q", tag.ID, tag.Slug)
		}
	}

	runItemCases(t, []itemCase{
		{
			name: "MissingArgs", method: "POST", call: "itemtags", body: `{"name":"bug"}`,
			want: http.StatusBadRequest,
		},
		{
			name: "BadSeq", method: "POST", call: "itemtags", slug: "foo", seq: "abc", body: `{}`,
			want: http.StatusBadRequest,
		},
		{
			name: "MethodNotAllowed", method: "GET", call: "itemtags", slug: "foo", seq: "1",
			want: http.StatusMethodNotAllowed,
		},
		{
			name: "ByID_Success", method: "POST", call: "itemtags", slug: "foo", seq: "1",
			body: `{"tag_id":"` + testTagID + `"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnRows(itemRows())
				m.ExpectQuery(`FROM tags WHERE user_id = \$1 AND id = \$2`).
					WithArgs(testUserID, testTagID).WillReturnRows(tagRows())
				m.ExpectExec(`INSERT INTO item_tags`).
					WithArgs(testItemID, testTagID, testUserID).WillReturnResult(sqlmock.NewResult(0, 1))
			},
			want:  http.StatusCreated,
			check: assertTagID,
		},
		{
			name: "BySlug_Success", method: "POST", call: "itemtags", slug: "foo", seq: "1",
			body: `{"tag_slug":"bug"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnRows(itemRows())
				m.ExpectQuery(`FROM tags WHERE user_id = \$1 AND slug = \$2`).
					WithArgs(testUserID, "bug").WillReturnRows(tagRows())
				m.ExpectExec(`INSERT INTO item_tags`).
					WithArgs(testItemID, testTagID, testUserID).WillReturnResult(sqlmock.NewResult(0, 1))
			},
			want:  http.StatusCreated,
			check: assertTagID,
		},
		{
			name: "ByName_Success", method: "POST", call: "itemtags", slug: "foo", seq: "1",
			body: `{"name":"Bug"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnRows(itemRows())
				m.ExpectQuery(`INSERT INTO tags`).
					WithArgs("bug", "Bug", "bug", sql.NullString{}, sql.NullString{}, sql.NullString{}, testUserID).
					WillReturnRows(tagRows())
				m.ExpectExec(`INSERT INTO item_tags`).
					WithArgs(testItemID, testTagID, testUserID).WillReturnResult(sqlmock.NewResult(0, 1))
			},
			want:  http.StatusCreated,
			check: assertTagID,
		},
		{
			name: "MissingTagSpec", method: "POST", call: "itemtags", slug: "foo", seq: "1", body: `{}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnRows(itemRows())
			},
			want: http.StatusBadRequest,
		},
		{
			name: "InvalidJSON", method: "POST", call: "itemtags", slug: "foo", seq: "1", body: `{bad`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnRows(itemRows())
			},
			want: http.StatusBadRequest,
		},
		{
			name: "ItemNotFound", method: "POST", call: "itemtags", slug: "foo", seq: "1", body: `{"tag_slug":"bug"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnError(sql.ErrNoRows)
			},
			want: http.StatusNotFound,
		},
		{
			name: "ItemLookupError", method: "POST", call: "itemtags", slug: "foo", seq: "1", body: `{"tag_slug":"bug"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
		{
			name: "TagNotFound", method: "POST", call: "itemtags", slug: "foo", seq: "1", body: `{"tag_slug":"missing"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnRows(itemRows())
				m.ExpectQuery(`FROM tags WHERE user_id = \$1 AND slug = \$2`).
					WithArgs(testUserID, "missing").WillReturnError(sql.ErrNoRows)
			},
			want: http.StatusNotFound,
		},
		{
			name: "TagLookupError", method: "POST", call: "itemtags", slug: "foo", seq: "1", body: `{"tag_slug":"bug"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnRows(itemRows())
				m.ExpectQuery(`FROM tags WHERE user_id = \$1 AND slug = \$2`).
					WithArgs(testUserID, "bug").WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
		{
			name: "InsertError", method: "POST", call: "itemtags", slug: "foo", seq: "1", body: `{"tag_slug":"bug"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnRows(itemRows())
				m.ExpectQuery(`FROM tags WHERE user_id = \$1 AND slug = \$2`).
					WithArgs(testUserID, "bug").WillReturnRows(tagRows())
				m.ExpectExec(`INSERT INTO item_tags`).
					WithArgs(testItemID, testTagID, testUserID).WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
		{
			name: "Forbidden", method: "POST", call: "itemtags", slug: "foo", seq: "1", body: `{"tag_slug":"bug"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) { mockProjectLookup(t, m, "other-owner") },
			want:  http.StatusForbidden,
		},
	})
}

func TestItems_DetachTag(t *testing.T) {
	runItemCases(t, []itemCase{
		{
			name: "Success", method: "DELETE", call: "itemtagbyid", slug: "foo", seq: "1", tagSlug: "bug",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnRows(itemRows())
				m.ExpectQuery(`FROM tags WHERE user_id = \$1 AND slug = \$2`).
					WithArgs(testUserID, "bug").WillReturnRows(tagRows())
				m.ExpectExec(`DELETE FROM item_tags`).
					WithArgs(testItemID, testTagID).WillReturnResult(sqlmock.NewResult(0, 1))
			},
			want: http.StatusNoContent,
		},
		{
			name: "MissingArgs", method: "DELETE", call: "itemtagbyid",
			want: http.StatusBadRequest,
		},
		{
			name: "BadSeq", method: "DELETE", call: "itemtagbyid", slug: "foo", seq: "abc", tagSlug: "bug",
			want: http.StatusBadRequest,
		},
		{
			name: "MethodNotAllowed", method: "POST", call: "itemtagbyid", slug: "foo", seq: "1", tagSlug: "bug",
			want: http.StatusMethodNotAllowed,
		},
		{
			name: "ItemNotFound", method: "DELETE", call: "itemtagbyid", slug: "foo", seq: "1", tagSlug: "bug",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnError(sql.ErrNoRows)
			},
			want: http.StatusNotFound,
		},
		{
			name: "ItemLookupError", method: "DELETE", call: "itemtagbyid", slug: "foo", seq: "1", tagSlug: "bug",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
		{
			name: "TagNotFound", method: "DELETE", call: "itemtagbyid", slug: "foo", seq: "1", tagSlug: "missing",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnRows(itemRows())
				m.ExpectQuery(`FROM tags WHERE user_id = \$1 AND slug = \$2`).
					WithArgs(testUserID, "missing").WillReturnError(sql.ErrNoRows)
			},
			want: http.StatusNotFound,
		},
		{
			name: "TagLookupError", method: "DELETE", call: "itemtagbyid", slug: "foo", seq: "1", tagSlug: "bug",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnRows(itemRows())
				m.ExpectQuery(`FROM tags WHERE user_id = \$1 AND slug = \$2`).
					WithArgs(testUserID, "bug").WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
		{
			name: "DeleteError", method: "DELETE", call: "itemtagbyid", slug: "foo", seq: "1", tagSlug: "bug",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`FROM items WHERE project_id`).WithArgs(testProjectID, 1).WillReturnRows(itemRows())
				m.ExpectQuery(`FROM tags WHERE user_id = \$1 AND slug = \$2`).
					WithArgs(testUserID, "bug").WillReturnRows(tagRows())
				m.ExpectExec(`DELETE FROM item_tags`).
					WithArgs(testItemID, testTagID).WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
		{
			name: "Forbidden", method: "DELETE", call: "itemtagbyid", slug: "foo", seq: "1", tagSlug: "bug",
			setup: func(t *testing.T, m sqlmock.Sqlmock) { mockProjectLookup(t, m, "other-owner") },
			want:  http.StatusForbidden,
		},
	})
}
