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

// projectUpdateSQL pins the full UPDATE statement so a loose `UPDATE projects`
// can't match a regressed column order or a dropped slug guard.
const projectUpdateSQL = `UPDATE projects\s+SET name = \$1,\s+slug = COALESCE\(NULLIF\(\$2, ''\), slug\),\s+description = \$3,\s+colour = \$4,\s+icon = \$5,\s+updated_at = NOW\(\)\s+WHERE id = \$6`

func projectRows() *sqlmock.Rows {
	return projectRowsOwnedBy(testUserID)
}

func projectRowsOwnedBy(owner string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "slug", "name", "description", "colour", "icon",
		"owner_id", "archived_at", "created_at", "updated_at",
	}).AddRow(testProjectID, "foo", "Foo", nil, nil, nil, owner, nil, time.Now(), time.Now())
}

// projectCase drives one subtest against a ProjectsHandler entrypoint. For
// byid/checkslug the identifier comes from the URL path / query string.
type projectCase struct {
	name   string
	call   string // collection | checkslug | byid
	method string
	path   string // full URL incl query; defaulted per call when empty
	body   string
	setup  func(t *testing.T, m sqlmock.Sqlmock)
	want   int
	check  func(t *testing.T, w *httptest.ResponseRecorder)
}

func runProjectCases(t *testing.T, cases []projectCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := newMockDB(t)
			if tc.setup != nil {
				tc.setup(t, mock)
			}
			h := NewProjectsHandler(db)

			path := tc.path
			if path == "" {
				switch tc.call {
				case "collection":
					path = "/v1/projects"
				case "checkslug":
					path = "/v1/projects/check-slug"
				}
			}
			r := authedReq(tc.method, path, tc.body)
			w := httptest.NewRecorder()

			switch tc.call {
			case "collection":
				h.HandleCollection(w, r)
			case "checkslug":
				h.HandleCheckSlug(w, r)
			case "byid":
				h.HandleByID(w, r)
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

func assertProject(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	var p data.Project
	decodeJSON(t, w, &p)
	if p.ID != testProjectID || p.Slug != "foo" {
		t.Errorf("project: got id=%q slug=%q", p.ID, p.Slug)
	}
}

func TestProjects_Collection(t *testing.T) {
	runProjectCases(t, []projectCase{
		{
			name: "MethodNotAllowed", call: "collection", method: "PUT",
			want: http.StatusMethodNotAllowed,
		},
		{
			name: "List_Success", call: "collection", method: "GET",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM projects p`).WithArgs(testUserID).WillReturnRows(projectRows())
			},
			want: http.StatusOK,
			check: func(t *testing.T, w *httptest.ResponseRecorder) {
				var ps []data.Project
				decodeJSON(t, w, &ps)
				if len(ps) != 1 || ps[0].ID != testProjectID || ps[0].Slug != "foo" {
					t.Errorf("unexpected list payload: %+v", ps)
				}
			},
		},
		{
			name: "List_DBError", call: "collection", method: "GET",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM projects p`).WithArgs(testUserID).WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
		{
			name: "Create_Success", call: "collection", method: "POST", body: `{"name":"Foo","slug":"foo"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectBegin()
				m.ExpectQuery(`INSERT INTO projects`).
					WithArgs("foo", "Foo", sql.NullString{}, sql.NullString{}, sql.NullString{}, testUserID).
					WillReturnRows(projectRows())
				m.ExpectExec(`INSERT INTO project_item_sequences`).
					WithArgs(testProjectID).WillReturnResult(sqlmock.NewResult(0, 1))
				m.ExpectCommit()
			},
			want:  http.StatusCreated,
			check: assertProject,
		},
		{
			name: "Create_InvalidJSON", call: "collection", method: "POST", body: `{nope`,
			want: http.StatusBadRequest,
		},
		{
			name: "Create_MissingName", call: "collection", method: "POST", body: `{"name":"  "}`,
			want: http.StatusBadRequest,
		},
		{
			name: "Create_InvalidSlug", call: "collection", method: "POST", body: `{"name":"Foo","slug":"BAD SLUG"}`,
			want: http.StatusBadRequest,
		},
		{
			name: "Create_SlugTaken", call: "collection", method: "POST", body: `{"name":"Foo","slug":"foo"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectBegin()
				m.ExpectQuery(`INSERT INTO projects`).
					WithArgs("foo", "Foo", sql.NullString{}, sql.NullString{}, sql.NullString{}, testUserID).
					WillReturnError(&pq.Error{Code: data.PGUniqueViolation})
				m.ExpectRollback()
			},
			want: http.StatusConflict,
		},
		{
			name: "Create_DBError", call: "collection", method: "POST", body: `{"name":"Foo","slug":"foo"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectBegin()
				m.ExpectQuery(`INSERT INTO projects`).
					WithArgs("foo", "Foo", sql.NullString{}, sql.NullString{}, sql.NullString{}, testUserID).
					WillReturnError(errors.New("boom"))
				m.ExpectRollback()
			},
			want: http.StatusInternalServerError,
		},
	})
}

func TestProjects_CheckSlug(t *testing.T) {
	runProjectCases(t, []projectCase{
		{
			name: "Available", call: "checkslug", method: "GET", path: "/v1/projects/check-slug?slug=foo",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`SELECT EXISTS`).
					WithArgs(testUserID, "foo").
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
			name: "Taken", call: "checkslug", method: "GET", path: "/v1/projects/check-slug?slug=foo",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`SELECT EXISTS`).
					WithArgs(testUserID, "foo").
					WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
			},
			want: http.StatusOK,
			check: func(t *testing.T, w *httptest.ResponseRecorder) {
				if !strings.Contains(w.Body.String(), `"available":false`) {
					t.Errorf("body: %s", w.Body.String())
				}
			},
		},
		{
			name: "InvalidSlug", call: "checkslug", method: "GET", path: "/v1/projects/check-slug?slug=BAD",
			want: http.StatusOK,
			check: func(t *testing.T, w *httptest.ResponseRecorder) {
				if !strings.Contains(w.Body.String(), `"available":false`) {
					t.Errorf("body: %s", w.Body.String())
				}
			},
		},
		{
			name: "DBError", call: "checkslug", method: "GET", path: "/v1/projects/check-slug?slug=foo",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`SELECT EXISTS`).
					WithArgs(testUserID, "foo").WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
		{
			name: "WrongMethod", call: "checkslug", method: "POST", path: "/v1/projects/check-slug?slug=foo",
			want: http.StatusMethodNotAllowed,
		},
	})
}

func TestProjects_ByID(t *testing.T) {
	runProjectCases(t, []projectCase{
		{
			name: "MissingID", call: "byid", method: "GET", path: "/v1/projects/",
			want: http.StatusBadRequest,
		},
		{
			name: "NestedPath", call: "byid", method: "GET", path: "/v1/projects/foo/items",
			want: http.StatusBadRequest,
		},
		{
			name: "MethodNotAllowed", call: "byid", method: "POST", path: "/v1/projects/foo",
			want: http.StatusMethodNotAllowed,
		},
		{
			name: "Get_Success", call: "byid", method: "GET", path: "/v1/projects/foo",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM projects p`).WithArgs(testUserID, "foo").WillReturnRows(projectRows())
			},
			want:  http.StatusOK,
			check: assertProject,
		},
		{
			name: "Get_NotFound", call: "byid", method: "GET", path: "/v1/projects/foo",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM projects p`).WithArgs(testUserID, "foo").WillReturnError(sql.ErrNoRows)
			},
			want: http.StatusNotFound,
		},
		{
			name: "Get_DBError", call: "byid", method: "GET", path: "/v1/projects/foo",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM projects p`).WithArgs(testUserID, "foo").WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
		{
			name: "Update_Success", call: "byid", method: "PATCH", path: "/v1/projects/foo", body: `{"name":"Foo2"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM projects p`).WithArgs(testUserID, "foo").WillReturnRows(projectRows())
				m.ExpectQuery(projectUpdateSQL).
					WithArgs("Foo2", "", sql.NullString{}, sql.NullString{}, sql.NullString{}, testProjectID).
					WillReturnRows(projectRows())
			},
			want:  http.StatusOK,
			check: assertProject,
		},
		{
			name: "Update_NotFound", call: "byid", method: "PATCH", path: "/v1/projects/foo", body: `{"name":"Foo2"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM projects p`).WithArgs(testUserID, "foo").WillReturnError(sql.ErrNoRows)
			},
			want: http.StatusNotFound,
		},
		{
			name: "Update_LookupError", call: "byid", method: "PATCH", path: "/v1/projects/foo", body: `{"name":"Foo2"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM projects p`).WithArgs(testUserID, "foo").WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
		{
			name: "Update_Forbidden", call: "byid", method: "PATCH", path: "/v1/projects/foo", body: `{"name":"Foo2"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM projects p`).WithArgs(testUserID, "foo").
					WillReturnRows(projectRowsOwnedBy("other-owner"))
			},
			want: http.StatusForbidden,
		},
		{
			name: "Update_InvalidJSON", call: "byid", method: "PATCH", path: "/v1/projects/foo", body: `{bad`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM projects p`).WithArgs(testUserID, "foo").WillReturnRows(projectRows())
			},
			want: http.StatusBadRequest,
		},
		{
			name: "Update_MissingName", call: "byid", method: "PATCH", path: "/v1/projects/foo", body: `{"name":"  "}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM projects p`).WithArgs(testUserID, "foo").WillReturnRows(projectRows())
			},
			want: http.StatusBadRequest,
		},
		{
			name: "Update_InvalidSlug", call: "byid", method: "PATCH", path: "/v1/projects/foo", body: `{"name":"Foo","slug":"BAD"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM projects p`).WithArgs(testUserID, "foo").WillReturnRows(projectRows())
			},
			want: http.StatusBadRequest,
		},
		{
			name: "Update_SlugTaken", call: "byid", method: "PATCH", path: "/v1/projects/foo", body: `{"name":"Foo","slug":"bar"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM projects p`).WithArgs(testUserID, "foo").WillReturnRows(projectRows())
				m.ExpectQuery(projectUpdateSQL).
					WithArgs("Foo", "bar", sql.NullString{}, sql.NullString{}, sql.NullString{}, testProjectID).
					WillReturnError(&pq.Error{Code: data.PGUniqueViolation})
			},
			want: http.StatusConflict,
		},
		{
			name: "Update_DBError", call: "byid", method: "PATCH", path: "/v1/projects/foo", body: `{"name":"Foo"}`,
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM projects p`).WithArgs(testUserID, "foo").WillReturnRows(projectRows())
				m.ExpectQuery(projectUpdateSQL).
					WithArgs("Foo", "", sql.NullString{}, sql.NullString{}, sql.NullString{}, testProjectID).
					WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
		{
			name: "Delete_Success", call: "byid", method: "DELETE", path: "/v1/projects/foo",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM projects p`).WithArgs(testUserID, "foo").WillReturnRows(projectRows())
				m.ExpectExec(`DELETE FROM projects`).WithArgs(testProjectID).WillReturnResult(sqlmock.NewResult(0, 1))
			},
			want: http.StatusNoContent,
		},
		{
			name: "Delete_NotFound", call: "byid", method: "DELETE", path: "/v1/projects/foo",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM projects p`).WithArgs(testUserID, "foo").WillReturnError(sql.ErrNoRows)
			},
			want: http.StatusNotFound,
		},
		{
			name: "Delete_LookupError", call: "byid", method: "DELETE", path: "/v1/projects/foo",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM projects p`).WithArgs(testUserID, "foo").WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
		{
			name: "Delete_Forbidden", call: "byid", method: "DELETE", path: "/v1/projects/foo",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM projects p`).WithArgs(testUserID, "foo").
					WillReturnRows(projectRowsOwnedBy("other-owner"))
			},
			want: http.StatusForbidden,
		},
		{
			name: "Delete_DBError", call: "byid", method: "DELETE", path: "/v1/projects/foo",
			setup: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(`FROM projects p`).WithArgs(testUserID, "foo").WillReturnRows(projectRows())
				m.ExpectExec(`DELETE FROM projects`).WithArgs(testProjectID).WillReturnError(errors.New("boom"))
			},
			want: http.StatusInternalServerError,
		},
	})
}
