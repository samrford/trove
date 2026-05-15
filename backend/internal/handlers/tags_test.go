package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"

	"trove/backend/internal/data"
)

// tagUpdateSQL pins the full UPDATE statement (column order + COALESCE/NULLIF
// slug guard) so a loose `UPDATE tags` can't match a regressed statement.
const tagUpdateSQL = `UPDATE tags\s+SET name = \$1,\s+name_normalised = \$2,\s+slug = COALESCE\(NULLIF\(\$3, ''\), slug\),\s+description = \$4,\s+icon = \$5,\s+colour = \$6,\s+updated_at = NOW\(\)\s+WHERE id = \$7`

func tagCountRows(itemCount int) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{
		"id", "slug", "name", "name_normalised", "description", "icon", "colour",
		"user_id", "group_id", "archived_at", "created_at", "updated_at",
		"item_count", "last_used_at",
	}).AddRow(testTagID, "bug", "Bug", "bug", nil, nil, nil, testUserID, nil, nil, now, now, itemCount, now)
}

// tagRowsOwnedBy returns a single tag row whose user_id is owner (pass nil for
// a group-owned / unowned tag).
func tagRowsOwnedBy(owner any) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "slug", "name", "name_normalised", "description", "icon", "colour",
		"user_id", "group_id", "archived_at", "created_at", "updated_at",
	}).AddRow(testTagID, "bug", "Bug", "bug", nil, nil, nil, owner, nil, nil, time.Now(), time.Now())
}

// tagCase drives one subtest against a TagsHandler entrypoint. For byid the
// slugOrID is parsed from path; for itemsfortag/tagsforproject it comes from
// the slug path value.
type tagCase struct {
	name   string
	call   string // collection | checkslug | byid | itemsfortag | tagsforproject
	method string
	path   string // full URL incl query; defaulted per call when empty
	slug   string // path value for itemsfortag / tagsforproject
	body   string
	setup  func(t *testing.T, m sqlmock.Sqlmock)
	want   int
	check  func(t *testing.T, w *httptest.ResponseRecorder)
}

func runTagCases(t *testing.T, cases []tagCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := newMockDB(t)
			if tc.setup != nil {
				tc.setup(t, mock)
			}
			h := NewTagsHandler(db)

			path := tc.path
			if path == "" {
				switch tc.call {
				case "collection":
					path = "/v1/tags"
				case "checkslug":
					path = "/v1/tags/check-slug"
				case "itemsfortag":
					path = "/v1/tags/x/items"
				case "tagsforproject":
					path = "/v1/projects/foo/tags"
				}
			}
			r := authedReq(tc.method, path, tc.body)
			if tc.slug != "" {
				r.SetPathValue("slug", tc.slug)
			}
			w := httptest.NewRecorder()

			switch tc.call {
			case "collection":
				h.HandleCollection(w, r)
			case "checkslug":
				h.HandleCheckSlug(w, r)
			case "byid":
				h.HandleByID(w, r)
			case "itemsfortag":
				h.HandleItemsForTag(w, r)
			case "tagsforproject":
				h.HandleTagsForProject(w, r)
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

func assertTag(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	var tag data.Tag
	decodeJSON(t, w, &tag)
	if tag.ID != testTagID || tag.Slug != "bug" {
		t.Errorf("tag: got id=%q slug=%q", tag.ID, tag.Slug)
	}
}

func TestTags_Collection(t *testing.T) {
	runTagCases(t, []tagCase{
		{
			name: "MethodNotAllowed", call: "collection", method: "PUT",
			want: http.StatusMethodNotAllowed,
		},
		{
			name: "List_Success", call: "collection", method: "GET",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM tags t\s+LEFT JOIN`).WithArgs(testUserID).WillReturnRows(tagCountRows(2))
			},
			want: http.StatusOK,
			check: func(t *testing.T, w *httptest.ResponseRecorder) {
				var tags []data.TagWithCount
				decodeJSON(t, w, &tags)
				if len(tags) != 1 || tags[0].Slug != "bug" || tags[0].ItemCount != 2 {
					t.Errorf("unexpected list payload: %+v", tags)
				}
			},
		},
		{
			name: "List_DBError", call: "collection", method: "GET",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM tags t`).WithArgs(testUserID).WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
		{
			name: "Create_Success", call: "collection", method: "POST", body: `{"name":"Bug","slug":"bug"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`INSERT INTO tags`).
					WithArgs("bug", "Bug", "bug", sql.NullString{}, sql.NullString{}, sql.NullString{}, testUserID).
					WillReturnRows(tagRows())
			},
			want:  http.StatusCreated,
			check: assertTag,
		},
		{
			name: "Create_AutoMerge", call: "collection", method: "POST", body: `{"name":"Bug","slug":"bug"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`INSERT INTO tags`).
					WithArgs("bug", "Bug", "bug", sql.NullString{}, sql.NullString{}, sql.NullString{}, testUserID).
					WillReturnError(&pq.Error{Code: data.PGUniqueViolation, Constraint: data.ConstraintTagsOwnerNormalisedUnique})
				m.ExpectQuery(`FROM tags WHERE user_id = \$1 AND name_normalised = \$2`).
					WithArgs(testUserID, "bug").WillReturnRows(tagRows())
			},
			// Auto-merge contract: 200 (not 201) with the *existing* tag body,
			// so the client can tell "new" from "you already had this".
			want:  http.StatusOK,
			check: assertTag,
		},
		{
			name: "Create_InvalidJSON", call: "collection", method: "POST", body: `{bad`,
			want: http.StatusBadRequest,
		},
		{
			name: "Create_MissingName", call: "collection", method: "POST", body: `{"name":"  "}`,
			want: http.StatusBadRequest,
		},
		{
			name: "Create_InvalidSlug", call: "collection", method: "POST", body: `{"name":"Bug","slug":"BAD"}`,
			want: http.StatusBadRequest,
		},
		{
			name: "Create_SlugTaken", call: "collection", method: "POST", body: `{"name":"Bug","slug":"bug"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`INSERT INTO tags`).
					WithArgs("bug", "Bug", "bug", sql.NullString{}, sql.NullString{}, sql.NullString{}, testUserID).
					WillReturnError(&pq.Error{Code: data.PGUniqueViolation, Constraint: data.ConstraintTagsOwnerSlugUnique})
			},
			want: http.StatusConflict,
		},
		{
			name: "Create_DBError", call: "collection", method: "POST", body: `{"name":"Bug","slug":"bug"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`INSERT INTO tags`).
					WithArgs("bug", "Bug", "bug", sql.NullString{}, sql.NullString{}, sql.NullString{}, testUserID).
					WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
	})
}

func TestTags_CheckSlug(t *testing.T) {
	runTagCases(t, []tagCase{
		{
			name: "Available", call: "checkslug", method: "GET", path: "/v1/tags/check-slug?slug=bug",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM tags WHERE user_id = \$1 AND slug = \$2`).
					WithArgs(testUserID, "bug").
					WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
			},
			want: http.StatusOK,
			check: func(t *testing.T, w *httptest.ResponseRecorder) {
				if !strings.Contains(w.Body.String(), `"available":true`) {
					t.Errorf("body: %s", w.Body.String())
				}
			},
		},
		{
			name: "InvalidSlug", call: "checkslug", method: "GET", path: "/v1/tags/check-slug?slug=BAD",
			want: http.StatusOK,
			check: func(t *testing.T, w *httptest.ResponseRecorder) {
				if !strings.Contains(w.Body.String(), `"available":false`) {
					t.Errorf("body: %s", w.Body.String())
				}
			},
		},
		{
			name: "DBError", call: "checkslug", method: "GET", path: "/v1/tags/check-slug?slug=bug",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM tags WHERE user_id`).
					WithArgs(testUserID, "bug").WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
		{
			name: "WrongMethod", call: "checkslug", method: "POST", path: "/v1/tags/check-slug?slug=bug",
			want: http.StatusMethodNotAllowed,
		},
	})
}

func TestTags_ByID(t *testing.T) {
	runTagCases(t, []tagCase{
		{
			name: "MissingID", call: "byid", method: "GET", path: "/v1/tags/",
			want: http.StatusBadRequest,
		},
		{
			name: "NestedPath", call: "byid", method: "GET", path: "/v1/tags/bug/items",
			want: http.StatusBadRequest,
		},
		{
			name: "MethodNotAllowed", call: "byid", method: "POST", path: "/v1/tags/bug",
			want: http.StatusMethodNotAllowed,
		},
		{
			name: "Get_Success", call: "byid", method: "GET", path: "/v1/tags/bug",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM tags WHERE user_id = \$1 AND slug = \$2`).
					WithArgs(testUserID, "bug").WillReturnRows(tagRows())
			},
			want:  http.StatusOK,
			check: assertTag,
		},
		{
			name: "Get_NotFound", call: "byid", method: "GET", path: "/v1/tags/bug",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM tags WHERE user_id`).WithArgs(testUserID, "bug").WillReturnError(sql.ErrNoRows)
			},
			want: http.StatusNotFound,
		},
		{
			name: "Get_DBError", call: "byid", method: "GET", path: "/v1/tags/bug",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM tags WHERE user_id`).WithArgs(testUserID, "bug").WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
		{
			name: "Update_Success", call: "byid", method: "PATCH", path: "/v1/tags/bug",
			body: `{"name":"Bug2","slug":"bug2"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM tags WHERE user_id = \$1 AND slug = \$2`).
					WithArgs(testUserID, "bug").WillReturnRows(tagRows())
				m.ExpectQuery(tagUpdateSQL).
					WithArgs("Bug2", "bug2", "bug2", sql.NullString{}, sql.NullString{}, sql.NullString{}, testTagID).
					WillReturnRows(tagRows())
			},
			want:  http.StatusOK,
			check: assertTag,
		},
		{
			name: "Update_NotFound", call: "byid", method: "PATCH", path: "/v1/tags/bug", body: `{"name":"Bug2"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM tags WHERE user_id`).WithArgs(testUserID, "bug").WillReturnError(sql.ErrNoRows)
			},
			want: http.StatusNotFound,
		},
		{
			name: "Update_LookupError", call: "byid", method: "PATCH", path: "/v1/tags/bug", body: `{"name":"Bug2"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM tags WHERE user_id`).WithArgs(testUserID, "bug").WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
		{
			name: "Update_Forbidden_OtherOwner", call: "byid", method: "PATCH", path: "/v1/tags/bug", body: `{"name":"Bug2"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM tags WHERE user_id`).WithArgs(testUserID, "bug").
					WillReturnRows(tagRowsOwnedBy("other-owner"))
			},
			want: http.StatusForbidden,
		},
		{
			// requireOwnedTag also rejects tags with a NULL user_id (group-owned
			// / unowned) — previously untested.
			name: "Update_Forbidden_Unowned", call: "byid", method: "PATCH", path: "/v1/tags/bug", body: `{"name":"Bug2"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM tags WHERE user_id`).WithArgs(testUserID, "bug").
					WillReturnRows(tagRowsOwnedBy(nil))
			},
			want: http.StatusForbidden,
		},
		{
			name: "Update_InvalidJSON", call: "byid", method: "PATCH", path: "/v1/tags/bug", body: `{bad`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM tags WHERE user_id`).WithArgs(testUserID, "bug").WillReturnRows(tagRows())
			},
			want: http.StatusBadRequest,
		},
		{
			name: "Update_MissingName", call: "byid", method: "PATCH", path: "/v1/tags/bug", body: `{"name":"  "}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM tags WHERE user_id`).WithArgs(testUserID, "bug").WillReturnRows(tagRows())
			},
			want: http.StatusBadRequest,
		},
		{
			name: "Update_InvalidSlug", call: "byid", method: "PATCH", path: "/v1/tags/bug", body: `{"name":"Bug","slug":"BAD"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM tags WHERE user_id`).WithArgs(testUserID, "bug").WillReturnRows(tagRows())
			},
			want: http.StatusBadRequest,
		},
		{
			name: "Update_SlugTaken", call: "byid", method: "PATCH", path: "/v1/tags/bug", body: `{"name":"Bug","slug":"bug2"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM tags WHERE user_id`).WithArgs(testUserID, "bug").WillReturnRows(tagRows())
				m.ExpectQuery(tagUpdateSQL).
					WithArgs("Bug", "bug", "bug2", sql.NullString{}, sql.NullString{}, sql.NullString{}, testTagID).
					WillReturnError(&pq.Error{Code: data.PGUniqueViolation, Constraint: data.ConstraintTagsOwnerSlugUnique})
			},
			want: http.StatusConflict,
		},
		{
			name: "Update_DBError", call: "byid", method: "PATCH", path: "/v1/tags/bug", body: `{"name":"Bug"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM tags WHERE user_id`).WithArgs(testUserID, "bug").WillReturnRows(tagRows())
				m.ExpectQuery(tagUpdateSQL).
					WithArgs("Bug", "bug", "", sql.NullString{}, sql.NullString{}, sql.NullString{}, testTagID).
					WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
		{
			name: "Delete_Success", call: "byid", method: "DELETE", path: "/v1/tags/bug",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM tags WHERE user_id`).WithArgs(testUserID, "bug").WillReturnRows(tagRows())
				m.ExpectExec(`DELETE FROM tags`).WithArgs(testTagID).WillReturnResult(sqlmock.NewResult(0, 1))
			},
			want: http.StatusNoContent,
		},
		{
			name: "Delete_NotFound", call: "byid", method: "DELETE", path: "/v1/tags/bug",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM tags WHERE user_id`).WithArgs(testUserID, "bug").WillReturnError(sql.ErrNoRows)
			},
			want: http.StatusNotFound,
		},
		{
			name: "Delete_DBError", call: "byid", method: "DELETE", path: "/v1/tags/bug",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM tags WHERE user_id`).WithArgs(testUserID, "bug").WillReturnRows(tagRows())
				m.ExpectExec(`DELETE FROM tags`).WithArgs(testTagID).WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
	})
}

func TestTags_ItemsForTag(t *testing.T) {
	runTagCases(t, []tagCase{
		{
			name: "Success", call: "itemsfortag", method: "GET", slug: "bug",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM tags WHERE user_id = \$1 AND slug = \$2`).
					WithArgs(testUserID, "bug").WillReturnRows(tagRows())
				m.ExpectQuery(`FROM items i\s+JOIN item_tags it ON it\.item_id = i\.id`).
					WithArgs(testTagID).WillReturnRows(itemRows())
			},
			want: http.StatusOK,
			check: func(t *testing.T, w *httptest.ResponseRecorder) {
				var items []data.Item
				decodeJSON(t, w, &items)
				if len(items) != 1 || items[0].ID != testItemID {
					t.Errorf("unexpected items: %+v", items)
				}
			},
		},
		{
			name: "MissingSlug", call: "itemsfortag", method: "GET",
			want: http.StatusBadRequest,
		},
		{
			name: "WrongMethod", call: "itemsfortag", method: "POST", slug: "bug",
			want: http.StatusMethodNotAllowed,
		},
		{
			name: "TagNotFound", call: "itemsfortag", method: "GET", slug: "bug",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM tags WHERE user_id`).WithArgs(testUserID, "bug").WillReturnError(sql.ErrNoRows)
			},
			want: http.StatusNotFound,
		},
		{
			name: "TagLookupError", call: "itemsfortag", method: "GET", slug: "bug",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM tags WHERE user_id`).WithArgs(testUserID, "bug").WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
		{
			name: "ListError", call: "itemsfortag", method: "GET", slug: "bug",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM tags WHERE user_id`).WithArgs(testUserID, "bug").WillReturnRows(tagRows())
				m.ExpectQuery(`FROM items i`).WithArgs(testTagID).WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
	})
}

func TestTags_TagsForProject(t *testing.T) {
	runTagCases(t, []tagCase{
		{
			name: "Success", call: "tagsforproject", method: "GET", slug: "foo",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`JOIN items i\s+ON i\.id = it\.item_id\s+WHERE i\.project_id = \$1`).
					WithArgs(testProjectID).WillReturnRows(tagCountRows(3))
			},
			want: http.StatusOK,
			check: func(t *testing.T, w *httptest.ResponseRecorder) {
				var tags []data.TagWithCount
				decodeJSON(t, w, &tags)
				if len(tags) != 1 || tags[0].ItemCount != 3 {
					t.Errorf("unexpected payload: %+v", tags)
				}
			},
		},
		{
			name: "WrongMethod", call: "tagsforproject", method: "POST", slug: "foo",
			want: http.StatusMethodNotAllowed,
		},
		{
			name: "MissingSlug", call: "tagsforproject", method: "GET",
			want: http.StatusBadRequest,
		},
		{
			name: "ProjectNotFound", call: "tagsforproject", method: "GET", slug: "foo",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM projects p`).WithArgs(testUserID, "foo").WillReturnError(sql.ErrNoRows)
			},
			want: http.StatusNotFound,
		},
		{
			name: "ProjectLookupError", call: "tagsforproject", method: "GET", slug: "foo",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM projects p`).WithArgs(testUserID, "foo").WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
		{
			name: "ListError", call: "tagsforproject", method: "GET", slug: "foo",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				mockProjectLookup(t, m, testUserID)
				m.ExpectQuery(`JOIN items i`).WithArgs(testProjectID).WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
	})
}
