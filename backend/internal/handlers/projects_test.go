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
)

func projectRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "slug", "name", "description", "colour", "icon",
		"owner_id", "archived_at", "created_at", "updated_at",
	}).AddRow(testProjectID, "foo", "Foo", nil, nil, nil, testUserID, nil, time.Now(), time.Now())
}

func TestProjects_HandleCollection_MethodNotAllowed(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewProjectsHandler(db)
	r := authedReq("PUT", "/v1/projects", "")
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("got %d", w.Code)
	}
}

func TestProjects_List_Success(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM projects p`).
		WithArgs(testUserID).
		WillReturnRows(projectRows())

	h := NewProjectsHandler(db)
	r := authedReq("GET", "/v1/projects", "")
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d", w.Code)
	}
}

func TestProjects_List_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM projects p`).
		WithArgs(testUserID).
		WillReturnError(errors.New("boom"))

	h := NewProjectsHandler(db)
	r := authedReq("GET", "/v1/projects", "")
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestProjects_Create_Success(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO projects`).
		WithArgs("foo", "Foo", sql.NullString{}, sql.NullString{}, sql.NullString{}, testUserID).
		WillReturnRows(projectRows())
	mock.ExpectExec(`INSERT INTO project_item_sequences`).
		WithArgs(testProjectID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := NewProjectsHandler(db)
	r := authedReq("POST", "/v1/projects", `{"name":"Foo","slug":"foo"}`)
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusCreated {
		t.Errorf("got %d body=%s", w.Code, w.Body.String())
	}
}

func TestProjects_Create_InvalidJSON(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewProjectsHandler(db)
	r := authedReq("POST", "/v1/projects", `{nope`)
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestProjects_Create_MissingName(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewProjectsHandler(db)
	r := authedReq("POST", "/v1/projects", `{"name":"  "}`)
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestProjects_Create_InvalidSlug(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewProjectsHandler(db)
	r := authedReq("POST", "/v1/projects", `{"name":"Foo","slug":"BAD SLUG"}`)
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestProjects_Create_SlugTaken(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO projects`).
		WithArgs("foo", "Foo", sql.NullString{}, sql.NullString{}, sql.NullString{}, testUserID).
		WillReturnError(&pq.Error{Code: "23505"})
	mock.ExpectRollback()

	h := NewProjectsHandler(db)
	r := authedReq("POST", "/v1/projects", `{"name":"Foo","slug":"foo"}`)
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusConflict {
		t.Errorf("got %d", w.Code)
	}
}

func TestProjects_Create_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO projects`).
		WithArgs("foo", "Foo", sql.NullString{}, sql.NullString{}, sql.NullString{}, testUserID).
		WillReturnError(errors.New("boom"))
	mock.ExpectRollback()

	h := NewProjectsHandler(db)
	r := authedReq("POST", "/v1/projects", `{"name":"Foo","slug":"foo"}`)
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestProjects_CheckSlug_Available(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs(testUserID, "foo").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	h := NewProjectsHandler(db)
	r := authedReq("GET", "/v1/projects/check-slug?slug=foo", "")
	w := httptest.NewRecorder()
	h.HandleCheckSlug(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d", w.Code)
	}
}

func TestProjects_CheckSlug_Taken(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs(testUserID, "foo").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	h := NewProjectsHandler(db)
	r := authedReq("GET", "/v1/projects/check-slug?slug=foo", "")
	w := httptest.NewRecorder()
	h.HandleCheckSlug(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d", w.Code)
	}
}

func TestProjects_CheckSlug_InvalidSlug(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewProjectsHandler(db)
	r := authedReq("GET", "/v1/projects/check-slug?slug=BAD", "")
	w := httptest.NewRecorder()
	h.HandleCheckSlug(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"available":false`) {
		t.Errorf("body: %s", w.Body.String())
	}
}

func TestProjects_CheckSlug_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs(testUserID, "foo").
		WillReturnError(errors.New("boom"))

	h := NewProjectsHandler(db)
	r := authedReq("GET", "/v1/projects/check-slug?slug=foo", "")
	w := httptest.NewRecorder()
	h.HandleCheckSlug(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestProjects_CheckSlug_WrongMethod(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewProjectsHandler(db)
	r := authedReq("POST", "/v1/projects/check-slug?slug=foo", "")
	w := httptest.NewRecorder()
	h.HandleCheckSlug(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("got %d", w.Code)
	}
}

func TestProjects_HandleByID_MissingID(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewProjectsHandler(db)
	r := authedReq("GET", "/v1/projects/", "")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestProjects_HandleByID_NestedPath(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewProjectsHandler(db)
	r := authedReq("GET", "/v1/projects/foo/items", "")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestProjects_HandleByID_MethodNotAllowed(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewProjectsHandler(db)
	r := authedReq("POST", "/v1/projects/foo", "")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("got %d", w.Code)
	}
}

func TestProjects_Get_Success(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM projects p`).
		WithArgs(testUserID, "foo").
		WillReturnRows(projectRows())

	h := NewProjectsHandler(db)
	r := authedReq("GET", "/v1/projects/foo", "")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d", w.Code)
	}
}

func TestProjects_Get_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM projects p`).
		WithArgs(testUserID, "foo").
		WillReturnError(sql.ErrNoRows)

	h := NewProjectsHandler(db)
	r := authedReq("GET", "/v1/projects/foo", "")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d", w.Code)
	}
}

func TestProjects_Get_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM projects p`).
		WithArgs(testUserID, "foo").
		WillReturnError(errors.New("boom"))

	h := NewProjectsHandler(db)
	r := authedReq("GET", "/v1/projects/foo", "")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestProjects_Update_Success(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM projects p`).
		WithArgs(testUserID, "foo").
		WillReturnRows(projectRows())
	mock.ExpectQuery(`UPDATE projects`).
		WithArgs("Foo2", "", sql.NullString{}, sql.NullString{}, sql.NullString{}, testProjectID).
		WillReturnRows(projectRows())

	h := NewProjectsHandler(db)
	r := authedReq("PATCH", "/v1/projects/foo", `{"name":"Foo2"}`)
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d body=%s", w.Code, w.Body.String())
	}
}

func TestProjects_Update_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM projects p`).
		WithArgs(testUserID, "foo").
		WillReturnError(sql.ErrNoRows)

	h := NewProjectsHandler(db)
	r := authedReq("PATCH", "/v1/projects/foo", `{"name":"Foo2"}`)
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d", w.Code)
	}
}

func TestProjects_Update_LookupError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM projects p`).
		WithArgs(testUserID, "foo").
		WillReturnError(errors.New("boom"))

	h := NewProjectsHandler(db)
	r := authedReq("PATCH", "/v1/projects/foo", `{"name":"Foo2"}`)
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestProjects_Update_Forbidden(t *testing.T) {
	db, mock := newMockDB(t)
	rows := sqlmock.NewRows([]string{
		"id", "slug", "name", "description", "colour", "icon",
		"owner_id", "archived_at", "created_at", "updated_at",
	}).AddRow(testProjectID, "foo", "Foo", nil, nil, nil, "other-owner", nil, time.Now(), time.Now())
	mock.ExpectQuery(`FROM projects p`).
		WithArgs(testUserID, "foo").
		WillReturnRows(rows)

	h := NewProjectsHandler(db)
	r := authedReq("PATCH", "/v1/projects/foo", `{"name":"Foo2"}`)
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("got %d", w.Code)
	}
}

func TestProjects_Update_InvalidJSON(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM projects p`).
		WithArgs(testUserID, "foo").
		WillReturnRows(projectRows())

	h := NewProjectsHandler(db)
	r := authedReq("PATCH", "/v1/projects/foo", `{bad`)
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestProjects_Update_MissingName(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM projects p`).
		WithArgs(testUserID, "foo").
		WillReturnRows(projectRows())

	h := NewProjectsHandler(db)
	r := authedReq("PATCH", "/v1/projects/foo", `{"name":"  "}`)
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestProjects_Update_InvalidSlug(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM projects p`).
		WithArgs(testUserID, "foo").
		WillReturnRows(projectRows())

	h := NewProjectsHandler(db)
	r := authedReq("PATCH", "/v1/projects/foo", `{"name":"Foo","slug":"BAD"}`)
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestProjects_Update_SlugTaken(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM projects p`).
		WithArgs(testUserID, "foo").
		WillReturnRows(projectRows())
	mock.ExpectQuery(`UPDATE projects`).
		WithArgs("Foo", "bar", sql.NullString{}, sql.NullString{}, sql.NullString{}, testProjectID).
		WillReturnError(&pq.Error{Code: "23505"})

	h := NewProjectsHandler(db)
	r := authedReq("PATCH", "/v1/projects/foo", `{"name":"Foo","slug":"bar"}`)
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusConflict {
		t.Errorf("got %d", w.Code)
	}
}

func TestProjects_Update_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM projects p`).
		WithArgs(testUserID, "foo").
		WillReturnRows(projectRows())
	mock.ExpectQuery(`UPDATE projects`).
		WithArgs("Foo", "", sql.NullString{}, sql.NullString{}, sql.NullString{}, testProjectID).
		WillReturnError(errors.New("boom"))

	h := NewProjectsHandler(db)
	r := authedReq("PATCH", "/v1/projects/foo", `{"name":"Foo"}`)
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestProjects_Delete_Success(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM projects p`).
		WithArgs(testUserID, "foo").
		WillReturnRows(projectRows())
	mock.ExpectExec(`DELETE FROM projects`).
		WithArgs(testProjectID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	h := NewProjectsHandler(db)
	r := authedReq("DELETE", "/v1/projects/foo", "")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusNoContent {
		t.Errorf("got %d", w.Code)
	}
}

func TestProjects_Delete_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM projects p`).
		WithArgs(testUserID, "foo").
		WillReturnError(sql.ErrNoRows)

	h := NewProjectsHandler(db)
	r := authedReq("DELETE", "/v1/projects/foo", "")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d", w.Code)
	}
}

func TestProjects_Delete_LookupError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM projects p`).
		WithArgs(testUserID, "foo").
		WillReturnError(errors.New("boom"))

	h := NewProjectsHandler(db)
	r := authedReq("DELETE", "/v1/projects/foo", "")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestProjects_Delete_Forbidden(t *testing.T) {
	db, mock := newMockDB(t)
	rows := sqlmock.NewRows([]string{
		"id", "slug", "name", "description", "colour", "icon",
		"owner_id", "archived_at", "created_at", "updated_at",
	}).AddRow(testProjectID, "foo", "Foo", nil, nil, nil, "other-owner", nil, time.Now(), time.Now())
	mock.ExpectQuery(`FROM projects p`).
		WithArgs(testUserID, "foo").
		WillReturnRows(rows)

	h := NewProjectsHandler(db)
	r := authedReq("DELETE", "/v1/projects/foo", "")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("got %d", w.Code)
	}
}

func TestProjects_Delete_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM projects p`).
		WithArgs(testUserID, "foo").
		WillReturnRows(projectRows())
	mock.ExpectExec(`DELETE FROM projects`).
		WithArgs(testProjectID).
		WillReturnError(errors.New("boom"))

	h := NewProjectsHandler(db)
	r := authedReq("DELETE", "/v1/projects/foo", "")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

