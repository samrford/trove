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

func tagReqWithSlug(method, path, body, slug string) *http.Request {
	r := authedReq(method, path, body)
	r.SetPathValue("slug", slug)
	return r
}

func TestTags_HandleCollection_MethodNotAllowed(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewTagsHandler(db)
	r := authedReq("PUT", "/v1/tags", "")
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_List_Success(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"id", "slug", "name", "name_normalised", "description", "icon", "colour",
		"user_id", "group_id", "archived_at", "created_at", "updated_at",
		"item_count", "last_used_at",
	}).AddRow(testTagID, "bug", "Bug", "bug", nil, nil, nil, testUserID, nil, nil, now, now, 2, now)
	mock.ExpectQuery(`FROM tags t\s+LEFT JOIN`).
		WithArgs(testUserID).
		WillReturnRows(rows)

	h := NewTagsHandler(db)
	r := authedReq("GET", "/v1/tags", "")
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_List_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM tags t`).
		WithArgs(testUserID).
		WillReturnError(errors.New("boom"))

	h := NewTagsHandler(db)
	r := authedReq("GET", "/v1/tags", "")
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_Create_Success(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`INSERT INTO tags`).
		WithArgs("bug", "Bug", "bug", sql.NullString{}, sql.NullString{}, sql.NullString{}, testUserID).
		WillReturnRows(tagRows())

	h := NewTagsHandler(db)
	r := authedReq("POST", "/v1/tags", `{"name":"Bug","slug":"bug"}`)
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusCreated {
		t.Errorf("got %d body=%s", w.Code, w.Body.String())
	}
}

func TestTags_Create_AutoMerge(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`INSERT INTO tags`).
		WithArgs("bug", "Bug", "bug", sql.NullString{}, sql.NullString{}, sql.NullString{}, testUserID).
		WillReturnError(&pq.Error{Code: data.PGUniqueViolation, Constraint: data.ConstraintTagsOwnerNormalisedUnique})
	mock.ExpectQuery(`FROM tags WHERE user_id = \$1 AND name_normalised = \$2`).
		WithArgs(testUserID, "bug").
		WillReturnRows(tagRows())

	h := NewTagsHandler(db)
	r := authedReq("POST", "/v1/tags", `{"name":"Bug","slug":"bug"}`)
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d body=%s", w.Code, w.Body.String())
	}
}

func TestTags_Create_InvalidJSON(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewTagsHandler(db)
	r := authedReq("POST", "/v1/tags", `{bad`)
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_Create_MissingName(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewTagsHandler(db)
	r := authedReq("POST", "/v1/tags", `{"name":"  "}`)
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_Create_InvalidSlug(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewTagsHandler(db)
	r := authedReq("POST", "/v1/tags", `{"name":"Bug","slug":"BAD"}`)
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_Create_SlugTaken(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`INSERT INTO tags`).
		WithArgs("bug", "Bug", "bug", sql.NullString{}, sql.NullString{}, sql.NullString{}, testUserID).
		WillReturnError(&pq.Error{Code: data.PGUniqueViolation, Constraint: data.ConstraintTagsOwnerSlugUnique})

	h := NewTagsHandler(db)
	r := authedReq("POST", "/v1/tags", `{"name":"Bug","slug":"bug"}`)
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusConflict {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_Create_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`INSERT INTO tags`).
		WithArgs("bug", "Bug", "bug", sql.NullString{}, sql.NullString{}, sql.NullString{}, testUserID).
		WillReturnError(errors.New("boom"))

	h := NewTagsHandler(db)
	r := authedReq("POST", "/v1/tags", `{"name":"Bug","slug":"bug"}`)
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_CheckSlug_Available(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM tags WHERE user_id = \$1 AND slug = \$2`).
		WithArgs(testUserID, "bug").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	h := NewTagsHandler(db)
	r := authedReq("GET", "/v1/tags/check-slug?slug=bug", "")
	w := httptest.NewRecorder()
	h.HandleCheckSlug(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_CheckSlug_InvalidSlug(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewTagsHandler(db)
	r := authedReq("GET", "/v1/tags/check-slug?slug=BAD", "")
	w := httptest.NewRecorder()
	h.HandleCheckSlug(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"available":false`) {
		t.Errorf("body: %s", w.Body.String())
	}
}

func TestTags_CheckSlug_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM tags WHERE user_id`).
		WithArgs(testUserID, "bug").
		WillReturnError(errors.New("boom"))

	h := NewTagsHandler(db)
	r := authedReq("GET", "/v1/tags/check-slug?slug=bug", "")
	w := httptest.NewRecorder()
	h.HandleCheckSlug(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_CheckSlug_WrongMethod(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewTagsHandler(db)
	r := authedReq("POST", "/v1/tags/check-slug?slug=bug", "")
	w := httptest.NewRecorder()
	h.HandleCheckSlug(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_HandleByID_MissingID(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewTagsHandler(db)
	r := authedReq("GET", "/v1/tags/", "")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_HandleByID_NestedPath(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewTagsHandler(db)
	r := authedReq("GET", "/v1/tags/bug/items", "")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_HandleByID_MethodNotAllowed(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewTagsHandler(db)
	r := authedReq("POST", "/v1/tags/bug", "")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_Get_Success(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM tags WHERE user_id = \$1 AND slug = \$2`).
		WithArgs(testUserID, "bug").
		WillReturnRows(tagRows())

	h := NewTagsHandler(db)
	r := authedReq("GET", "/v1/tags/bug", "")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_Get_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM tags WHERE user_id`).
		WithArgs(testUserID, "bug").
		WillReturnError(sql.ErrNoRows)

	h := NewTagsHandler(db)
	r := authedReq("GET", "/v1/tags/bug", "")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_Get_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM tags WHERE user_id`).
		WithArgs(testUserID, "bug").
		WillReturnError(errors.New("boom"))

	h := NewTagsHandler(db)
	r := authedReq("GET", "/v1/tags/bug", "")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_Update_Success(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM tags WHERE user_id = \$1 AND slug = \$2`).
		WithArgs(testUserID, "bug").
		WillReturnRows(tagRows())
	mock.ExpectQuery(`UPDATE tags`).
		WithArgs("Bug2", "bug2", "bug2", sql.NullString{}, sql.NullString{}, sql.NullString{}, testTagID).
		WillReturnRows(tagRows())

	h := NewTagsHandler(db)
	r := authedReq("PATCH", "/v1/tags/bug", `{"name":"Bug2","slug":"bug2"}`)
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d body=%s", w.Code, w.Body.String())
	}
}

func TestTags_Update_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM tags WHERE user_id`).
		WithArgs(testUserID, "bug").
		WillReturnError(sql.ErrNoRows)

	h := NewTagsHandler(db)
	r := authedReq("PATCH", "/v1/tags/bug", `{"name":"Bug2"}`)
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_Update_LookupError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM tags WHERE user_id`).
		WithArgs(testUserID, "bug").
		WillReturnError(errors.New("boom"))

	h := NewTagsHandler(db)
	r := authedReq("PATCH", "/v1/tags/bug", `{"name":"Bug2"}`)
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_Update_Forbidden(t *testing.T) {
	db, mock := newMockDB(t)
	rows := sqlmock.NewRows([]string{
		"id", "slug", "name", "name_normalised", "description", "icon", "colour",
		"user_id", "group_id", "archived_at", "created_at", "updated_at",
	}).AddRow(testTagID, "bug", "Bug", "bug", nil, nil, nil, "other-owner", nil, nil, time.Now(), time.Now())
	mock.ExpectQuery(`FROM tags WHERE user_id`).
		WithArgs(testUserID, "bug").
		WillReturnRows(rows)

	h := NewTagsHandler(db)
	r := authedReq("PATCH", "/v1/tags/bug", `{"name":"Bug2"}`)
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_Update_InvalidJSON(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM tags WHERE user_id`).
		WithArgs(testUserID, "bug").
		WillReturnRows(tagRows())

	h := NewTagsHandler(db)
	r := authedReq("PATCH", "/v1/tags/bug", `{bad`)
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_Update_MissingName(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM tags WHERE user_id`).
		WithArgs(testUserID, "bug").
		WillReturnRows(tagRows())

	h := NewTagsHandler(db)
	r := authedReq("PATCH", "/v1/tags/bug", `{"name":"  "}`)
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_Update_InvalidSlug(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM tags WHERE user_id`).
		WithArgs(testUserID, "bug").
		WillReturnRows(tagRows())

	h := NewTagsHandler(db)
	r := authedReq("PATCH", "/v1/tags/bug", `{"name":"Bug","slug":"BAD"}`)
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_Update_SlugTaken(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM tags WHERE user_id`).
		WithArgs(testUserID, "bug").
		WillReturnRows(tagRows())
	mock.ExpectQuery(`UPDATE tags`).
		WithArgs("Bug", "bug", "bug2", sql.NullString{}, sql.NullString{}, sql.NullString{}, testTagID).
		WillReturnError(&pq.Error{Code: data.PGUniqueViolation, Constraint: data.ConstraintTagsOwnerSlugUnique})

	h := NewTagsHandler(db)
	r := authedReq("PATCH", "/v1/tags/bug", `{"name":"Bug","slug":"bug2"}`)
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusConflict {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_Update_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM tags WHERE user_id`).
		WithArgs(testUserID, "bug").
		WillReturnRows(tagRows())
	mock.ExpectQuery(`UPDATE tags`).
		WithArgs("Bug", "bug", "", sql.NullString{}, sql.NullString{}, sql.NullString{}, testTagID).
		WillReturnError(errors.New("boom"))

	h := NewTagsHandler(db)
	r := authedReq("PATCH", "/v1/tags/bug", `{"name":"Bug"}`)
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_Delete_Success(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM tags WHERE user_id`).
		WithArgs(testUserID, "bug").
		WillReturnRows(tagRows())
	mock.ExpectExec(`DELETE FROM tags`).
		WithArgs(testTagID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	h := NewTagsHandler(db)
	r := authedReq("DELETE", "/v1/tags/bug", "")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusNoContent {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_Delete_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM tags WHERE user_id`).
		WithArgs(testUserID, "bug").
		WillReturnError(sql.ErrNoRows)

	h := NewTagsHandler(db)
	r := authedReq("DELETE", "/v1/tags/bug", "")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_Delete_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM tags WHERE user_id`).
		WithArgs(testUserID, "bug").
		WillReturnRows(tagRows())
	mock.ExpectExec(`DELETE FROM tags`).
		WithArgs(testTagID).
		WillReturnError(errors.New("boom"))

	h := NewTagsHandler(db)
	r := authedReq("DELETE", "/v1/tags/bug", "")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

// HandleItemsForTag

func TestTags_HandleItemsForTag_Success(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM tags WHERE user_id = \$1 AND slug = \$2`).
		WithArgs(testUserID, "bug").
		WillReturnRows(tagRows())
	mock.ExpectQuery(`FROM items i\s+JOIN item_tags it ON it\.item_id = i\.id`).
		WithArgs(testTagID).
		WillReturnRows(itemRows())

	h := NewTagsHandler(db)
	r := tagReqWithSlug("GET", "/v1/tags/bug/items", "", "bug")
	w := httptest.NewRecorder()
	h.HandleItemsForTag(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d body=%s", w.Code, w.Body.String())
	}
}

func TestTags_HandleItemsForTag_MissingSlug(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewTagsHandler(db)
	r := authedReq("GET", "/v1/tags//items", "")
	w := httptest.NewRecorder()
	h.HandleItemsForTag(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_HandleItemsForTag_WrongMethod(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewTagsHandler(db)
	r := tagReqWithSlug("POST", "/v1/tags/bug/items", "", "bug")
	w := httptest.NewRecorder()
	h.HandleItemsForTag(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_HandleItemsForTag_TagNotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM tags WHERE user_id`).
		WithArgs(testUserID, "bug").
		WillReturnError(sql.ErrNoRows)

	h := NewTagsHandler(db)
	r := tagReqWithSlug("GET", "/v1/tags/bug/items", "", "bug")
	w := httptest.NewRecorder()
	h.HandleItemsForTag(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_HandleItemsForTag_TagLookupError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM tags WHERE user_id`).
		WithArgs(testUserID, "bug").
		WillReturnError(errors.New("boom"))

	h := NewTagsHandler(db)
	r := tagReqWithSlug("GET", "/v1/tags/bug/items", "", "bug")
	w := httptest.NewRecorder()
	h.HandleItemsForTag(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_HandleItemsForTag_ListError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM tags WHERE user_id`).
		WithArgs(testUserID, "bug").
		WillReturnRows(tagRows())
	mock.ExpectQuery(`FROM items i`).
		WithArgs(testTagID).
		WillReturnError(errors.New("boom"))

	h := NewTagsHandler(db)
	r := tagReqWithSlug("GET", "/v1/tags/bug/items", "", "bug")
	w := httptest.NewRecorder()
	h.HandleItemsForTag(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

// HandleTagsForProject

func TestTags_HandleTagsForProject_Success(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	rows := sqlmock.NewRows([]string{
		"id", "slug", "name", "name_normalised", "description", "icon", "colour",
		"user_id", "group_id", "archived_at", "created_at", "updated_at",
		"item_count", "last_used_at",
	}).AddRow(testTagID, "bug", "Bug", "bug", nil, nil, nil, testUserID, nil, nil, time.Now(), time.Now(), 3, time.Now())
	mock.ExpectQuery(`JOIN items i\s+ON i\.id = it\.item_id\s+WHERE i\.project_id = \$1`).
		WithArgs(testProjectID).
		WillReturnRows(rows)

	h := NewTagsHandler(db)
	r := tagReqWithSlug("GET", "/v1/projects/foo/tags", "", "foo")
	w := httptest.NewRecorder()
	h.HandleTagsForProject(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d body=%s", w.Code, w.Body.String())
	}
}

func TestTags_HandleTagsForProject_WrongMethod(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewTagsHandler(db)
	r := tagReqWithSlug("POST", "/v1/projects/foo/tags", "", "foo")
	w := httptest.NewRecorder()
	h.HandleTagsForProject(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_HandleTagsForProject_MissingSlug(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewTagsHandler(db)
	r := authedReq("GET", "/v1/projects//tags", "")
	w := httptest.NewRecorder()
	h.HandleTagsForProject(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_HandleTagsForProject_ProjectNotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM projects p`).
		WithArgs(testUserID, "foo").
		WillReturnError(sql.ErrNoRows)

	h := NewTagsHandler(db)
	r := tagReqWithSlug("GET", "/v1/projects/foo/tags", "", "foo")
	w := httptest.NewRecorder()
	h.HandleTagsForProject(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_HandleTagsForProject_ProjectLookupError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM projects p`).
		WithArgs(testUserID, "foo").
		WillReturnError(errors.New("boom"))

	h := NewTagsHandler(db)
	r := tagReqWithSlug("GET", "/v1/projects/foo/tags", "", "foo")
	w := httptest.NewRecorder()
	h.HandleTagsForProject(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestTags_HandleTagsForProject_ListError(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`JOIN items i`).
		WithArgs(testProjectID).
		WillReturnError(errors.New("boom"))

	h := NewTagsHandler(db)
	r := tagReqWithSlug("GET", "/v1/projects/foo/tags", "", "foo")
	w := httptest.NewRecorder()
	h.HandleTagsForProject(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}
