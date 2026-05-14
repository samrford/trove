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

// kindTaskBody / kindAllBody build JSON request bodies referencing the typed
// enum constants, so a renamed enum value breaks these tests rather than
// silently shipping stale strings.
func kindTaskBody(title string) string {
	return fmt.Sprintf(`{"kind":%q,"title":%q}`, string(data.ItemKindTask), title)
}

func itemRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "project_id", "sequence", "kind", "status", "title", "body",
		"position", "creator_id", "created_at", "updated_at",
	}).AddRow(testItemID, testProjectID, 1, string(data.ItemKindTask), string(data.ItemStatusOpen), "T", nil, 1.0, testUserID, time.Now(), time.Now())
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

func itemReq(method, path, body, slug, seq string) *http.Request {
	r := authedReq(method, path, body)
	r.SetPathValue("slug", slug)
	if seq != "" {
		r.SetPathValue("seq", seq)
	}
	return r
}

func TestItems_HandleCollection_MissingSlug(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewItemsHandler(db)
	r := authedReq("GET", "/v1/projects//items", "")
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_HandleCollection_MethodNotAllowed(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewItemsHandler(db)
	r := itemReq("PUT", "/v1/projects/foo/items", "", "foo", "")
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_List_Success(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items i`).
		WithArgs(testProjectID).
		WillReturnRows(itemRows())
	mock.ExpectQuery(`WHERE it\.item_id = ANY`).
		WithArgs(pq.Array([]string{testItemID})).
		WillReturnRows(sqlmock.NewRows([]string{
			"item_id",
			"id", "slug", "name", "name_normalised", "description", "icon", "colour",
			"user_id", "group_id", "archived_at", "created_at", "updated_at",
		}).AddRow(testItemID, testTagID, "bug", "Bug", "bug", nil, nil, nil, testUserID, nil, nil, time.Now(), time.Now()))

	h := NewItemsHandler(db)
	r := itemReq("GET", "/v1/projects/foo/items", "", "foo", "")
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_List_ProjectNotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM projects p`).
		WithArgs(testUserID, "foo").
		WillReturnError(sql.ErrNoRows)

	h := NewItemsHandler(db)
	r := itemReq("GET", "/v1/projects/foo/items", "", "foo", "")
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_List_ProjectLookupError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`FROM projects p`).
		WithArgs(testUserID, "foo").
		WillReturnError(errors.New("boom"))

	h := NewItemsHandler(db)
	r := itemReq("GET", "/v1/projects/foo/items", "", "foo", "")
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_List_BadKind(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)

	h := NewItemsHandler(db)
	r := itemReq("GET", "/v1/projects/foo/items?kind=nope", "", "foo", "")
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_List_BadStatus(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)

	h := NewItemsHandler(db)
	r := itemReq("GET", "/v1/projects/foo/items?status=nope", "", "foo", "")
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_List_BadTagMode(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)

	h := NewItemsHandler(db)
	r := itemReq("GET", "/v1/projects/foo/items?tag_mode=xor", "", "foo", "")
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_List_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items i`).
		WithArgs(testProjectID).
		WillReturnError(errors.New("boom"))

	h := NewItemsHandler(db)
	r := itemReq("GET", "/v1/projects/foo/items", "", "foo", "")
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_List_TagsLookupError(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items i`).
		WithArgs(testProjectID).
		WillReturnRows(itemRows())
	mock.ExpectQuery(`WHERE it\.item_id = ANY`).
		WithArgs(pq.Array([]string{testItemID})).
		WillReturnError(errors.New("boom"))

	h := NewItemsHandler(db)
	r := itemReq("GET", "/v1/projects/foo/items", "", "foo", "")
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_Create_Success(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectBegin()
	mock.ExpectQuery(`UPDATE project_item_sequences`).
		WithArgs(testProjectID).
		WillReturnRows(sqlmock.NewRows([]string{"seq"}).AddRow(1))
	mock.ExpectQuery(`INSERT INTO items`).
		WithArgs(testProjectID, 1, string(data.ItemKindTask), "T", sql.NullString{}, testUserID).
		WillReturnRows(itemRows())
	mock.ExpectCommit()

	h := NewItemsHandler(db)
	r := itemReq("POST", "/v1/projects/foo/items", kindTaskBody("T"), "foo", "")
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusCreated {
		t.Errorf("got %d body=%s", w.Code, w.Body.String())
	}
}

func TestItems_Create_Forbidden(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, "other-owner")

	h := NewItemsHandler(db)
	r := itemReq("POST", "/v1/projects/foo/items", kindTaskBody("T"), "foo", "")
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_Create_InvalidJSON(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)

	h := NewItemsHandler(db)
	r := itemReq("POST", "/v1/projects/foo/items", `{bad`, "foo", "")
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_Create_MissingTitle(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)

	h := NewItemsHandler(db)
	r := itemReq("POST", "/v1/projects/foo/items", kindTaskBody("   "), "foo", "")
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_Create_BadKind(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)

	h := NewItemsHandler(db)
	r := itemReq("POST", "/v1/projects/foo/items", `{"kind":"x","title":"T"}`, "foo", "")
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_Create_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectBegin().WillReturnError(errors.New("nope"))

	h := NewItemsHandler(db)
	r := itemReq("POST", "/v1/projects/foo/items", kindTaskBody("T"), "foo", "")
	w := httptest.NewRecorder()
	h.HandleCollection(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_HandleByID_MissingSeq(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewItemsHandler(db)
	r := itemReq("GET", "/v1/projects/foo/items/", "", "foo", "")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_HandleByID_BadSeq(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewItemsHandler(db)
	r := itemReq("GET", "/v1/projects/foo/items/abc", "", "foo", "abc")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_HandleByID_MethodNotAllowed(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewItemsHandler(db)
	r := itemReq("PUT", "/v1/projects/foo/items/1", "", "foo", "1")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_Get_Success(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id = \$1 AND sequence = \$2`).
		WithArgs(testProjectID, 1).
		WillReturnRows(itemRows())
	mock.ExpectQuery(`FROM tags t\s+JOIN item_tags`).
		WithArgs(testItemID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "slug", "name", "name_normalised", "description", "icon", "colour",
			"user_id", "group_id", "archived_at", "created_at", "updated_at",
		}))

	h := NewItemsHandler(db)
	r := itemReq("GET", "/v1/projects/foo/items/1", "", "foo", "1")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_Get_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnError(sql.ErrNoRows)

	h := NewItemsHandler(db)
	r := itemReq("GET", "/v1/projects/foo/items/1", "", "foo", "1")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_Get_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnError(errors.New("boom"))

	h := NewItemsHandler(db)
	r := itemReq("GET", "/v1/projects/foo/items/1", "", "foo", "1")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_Get_TagsError(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnRows(itemRows())
	mock.ExpectQuery(`FROM tags t\s+JOIN item_tags`).
		WithArgs(testItemID).
		WillReturnError(errors.New("boom"))

	h := NewItemsHandler(db)
	r := itemReq("GET", "/v1/projects/foo/items/1", "", "foo", "1")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_Update_Success(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnRows(itemRows())
	mock.ExpectQuery(`UPDATE items SET`).
		WithArgs("Updated", testItemID).
		WillReturnRows(itemRows())
	mock.ExpectQuery(`FROM tags t\s+JOIN item_tags`).
		WithArgs(testItemID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "slug", "name", "name_normalised", "description", "icon", "colour",
			"user_id", "group_id", "archived_at", "created_at", "updated_at",
		}))

	h := NewItemsHandler(db)
	r := itemReq("PATCH", "/v1/projects/foo/items/1", `{"title":"Updated"}`, "foo", "1")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d body=%s", w.Code, w.Body.String())
	}
}

func TestItems_Update_ClearBody(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnRows(itemRows())
	mock.ExpectQuery(`UPDATE items SET .*body`).
		WithArgs(sql.NullString{}, testItemID).
		WillReturnRows(itemRows())
	mock.ExpectQuery(`FROM tags t\s+JOIN item_tags`).
		WithArgs(testItemID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "slug", "name", "name_normalised", "description", "icon", "colour",
			"user_id", "group_id", "archived_at", "created_at", "updated_at",
		}))

	h := NewItemsHandler(db)
	r := itemReq("PATCH", "/v1/projects/foo/items/1", `{"body":"   "}`, "foo", "1")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d body=%s", w.Code, w.Body.String())
	}
}

func TestItems_Update_AllFields(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnRows(itemRows())
	mock.ExpectQuery(`UPDATE items SET`).
		WithArgs("New", "body", string(data.ItemKindTask), string(data.ItemStatusDone), 1.5, testItemID).
		WillReturnRows(itemRows())
	mock.ExpectQuery(`FROM tags t\s+JOIN item_tags`).
		WithArgs(testItemID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "slug", "name", "name_normalised", "description", "icon", "colour",
			"user_id", "group_id", "archived_at", "created_at", "updated_at",
		}))

	h := NewItemsHandler(db)
	r := itemReq("PATCH", "/v1/projects/foo/items/1",
		fmt.Sprintf(`{"title":"New","body":"body","kind":%q,"status":%q,"position":1.5}`,
			string(data.ItemKindTask), string(data.ItemStatusDone)),
		"foo", "1")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d body=%s", w.Code, w.Body.String())
	}
}

func TestItems_Update_ItemNotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnError(sql.ErrNoRows)

	h := NewItemsHandler(db)
	r := itemReq("PATCH", "/v1/projects/foo/items/1", `{"title":"x"}`, "foo", "1")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_Update_ItemLookupError(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnError(errors.New("boom"))

	h := NewItemsHandler(db)
	r := itemReq("PATCH", "/v1/projects/foo/items/1", `{"title":"x"}`, "foo", "1")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_Update_InvalidJSON(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnRows(itemRows())

	h := NewItemsHandler(db)
	r := itemReq("PATCH", "/v1/projects/foo/items/1", `{bad`, "foo", "1")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_Update_EmptyTitle(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnRows(itemRows())

	h := NewItemsHandler(db)
	r := itemReq("PATCH", "/v1/projects/foo/items/1", `{"title":"  "}`, "foo", "1")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_Update_BadKind(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnRows(itemRows())

	h := NewItemsHandler(db)
	r := itemReq("PATCH", "/v1/projects/foo/items/1", `{"kind":"nope"}`, "foo", "1")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_Update_BadStatus(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnRows(itemRows())

	h := NewItemsHandler(db)
	r := itemReq("PATCH", "/v1/projects/foo/items/1", `{"status":"nope"}`, "foo", "1")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_Update_UpdateError(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnRows(itemRows())
	mock.ExpectQuery(`UPDATE items SET`).
		WithArgs("x", testItemID).
		WillReturnError(errors.New("boom"))

	h := NewItemsHandler(db)
	r := itemReq("PATCH", "/v1/projects/foo/items/1", `{"title":"x"}`, "foo", "1")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_Update_TagsError(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnRows(itemRows())
	mock.ExpectQuery(`UPDATE items SET`).
		WithArgs("x", testItemID).
		WillReturnRows(itemRows())
	mock.ExpectQuery(`FROM tags t\s+JOIN item_tags`).
		WithArgs(testItemID).
		WillReturnError(errors.New("boom"))

	h := NewItemsHandler(db)
	r := itemReq("PATCH", "/v1/projects/foo/items/1", `{"title":"x"}`, "foo", "1")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_Update_Forbidden(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, "other-owner")

	h := NewItemsHandler(db)
	r := itemReq("PATCH", "/v1/projects/foo/items/1", `{"title":"x"}`, "foo", "1")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_Delete_Success(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnRows(itemRows())
	mock.ExpectExec(`DELETE FROM items`).
		WithArgs(testItemID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	h := NewItemsHandler(db)
	r := itemReq("DELETE", "/v1/projects/foo/items/1", "", "foo", "1")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusNoContent {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_Delete_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnError(sql.ErrNoRows)

	h := NewItemsHandler(db)
	r := itemReq("DELETE", "/v1/projects/foo/items/1", "", "foo", "1")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_Delete_LookupError(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnError(errors.New("boom"))

	h := NewItemsHandler(db)
	r := itemReq("DELETE", "/v1/projects/foo/items/1", "", "foo", "1")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_Delete_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnRows(itemRows())
	mock.ExpectExec(`DELETE FROM items`).
		WithArgs(testItemID).
		WillReturnError(errors.New("boom"))

	h := NewItemsHandler(db)
	r := itemReq("DELETE", "/v1/projects/foo/items/1", "", "foo", "1")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_Delete_Forbidden(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, "other-owner")

	h := NewItemsHandler(db)
	r := itemReq("DELETE", "/v1/projects/foo/items/1", "", "foo", "1")
	w := httptest.NewRecorder()
	h.HandleByID(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("got %d", w.Code)
	}
}

// HandleItemTags

func TestItems_HandleItemTags_MissingArgs(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewItemsHandler(db)
	r := authedReq("POST", "/v1/projects/foo/items/1/tags", `{"name":"bug"}`)
	w := httptest.NewRecorder()
	h.HandleItemTags(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_HandleItemTags_BadSeq(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewItemsHandler(db)
	r := itemReq("POST", "/v1/projects/foo/items/abc/tags", `{}`, "foo", "abc")
	w := httptest.NewRecorder()
	h.HandleItemTags(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_HandleItemTags_MethodNotAllowed(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewItemsHandler(db)
	r := itemReq("GET", "/v1/projects/foo/items/1/tags", "", "foo", "1")
	w := httptest.NewRecorder()
	h.HandleItemTags(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("got %d", w.Code)
	}
}

func tagRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "slug", "name", "name_normalised", "description", "icon", "colour",
		"user_id", "group_id", "archived_at", "created_at", "updated_at",
	}).AddRow(testTagID, "bug", "Bug", "bug", nil, nil, nil, testUserID, nil, nil, time.Now(), time.Now())
}

func TestItems_AttachTag_ByID_Success(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnRows(itemRows())
	mock.ExpectQuery(`FROM tags WHERE user_id = \$1 AND id = \$2`).
		WithArgs(testUserID, testTagID).
		WillReturnRows(tagRows())
	mock.ExpectExec(`INSERT INTO item_tags`).
		WithArgs(testItemID, testTagID, testUserID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	h := NewItemsHandler(db)
	r := itemReq("POST", "/v1/projects/foo/items/1/tags",
		`{"tag_id":"`+testTagID+`"}`, "foo", "1")
	w := httptest.NewRecorder()
	h.HandleItemTags(w, r)
	if w.Code != http.StatusCreated {
		t.Errorf("got %d body=%s", w.Code, w.Body.String())
	}
}

func TestItems_AttachTag_BySlug_Success(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnRows(itemRows())
	mock.ExpectQuery(`FROM tags WHERE user_id = \$1 AND slug = \$2`).
		WithArgs(testUserID, "bug").
		WillReturnRows(tagRows())
	mock.ExpectExec(`INSERT INTO item_tags`).
		WithArgs(testItemID, testTagID, testUserID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	h := NewItemsHandler(db)
	r := itemReq("POST", "/v1/projects/foo/items/1/tags",
		`{"tag_slug":"bug"}`, "foo", "1")
	w := httptest.NewRecorder()
	h.HandleItemTags(w, r)
	if w.Code != http.StatusCreated {
		t.Errorf("got %d body=%s", w.Code, w.Body.String())
	}
}

func TestItems_AttachTag_ByName_Success(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnRows(itemRows())
	mock.ExpectQuery(`INSERT INTO tags`).
		WithArgs("bug", "Bug", "bug", sql.NullString{}, sql.NullString{}, sql.NullString{}, testUserID).
		WillReturnRows(tagRows())
	mock.ExpectExec(`INSERT INTO item_tags`).
		WithArgs(testItemID, testTagID, testUserID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	h := NewItemsHandler(db)
	r := itemReq("POST", "/v1/projects/foo/items/1/tags",
		`{"name":"Bug"}`, "foo", "1")
	w := httptest.NewRecorder()
	h.HandleItemTags(w, r)
	if w.Code != http.StatusCreated {
		t.Errorf("got %d body=%s", w.Code, w.Body.String())
	}
}

func TestItems_AttachTag_MissingTagSpec(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnRows(itemRows())

	h := NewItemsHandler(db)
	r := itemReq("POST", "/v1/projects/foo/items/1/tags", `{}`, "foo", "1")
	w := httptest.NewRecorder()
	h.HandleItemTags(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_AttachTag_InvalidJSON(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnRows(itemRows())

	h := NewItemsHandler(db)
	r := itemReq("POST", "/v1/projects/foo/items/1/tags", `{bad`, "foo", "1")
	w := httptest.NewRecorder()
	h.HandleItemTags(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_AttachTag_ItemNotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnError(sql.ErrNoRows)

	h := NewItemsHandler(db)
	r := itemReq("POST", "/v1/projects/foo/items/1/tags",
		`{"tag_slug":"bug"}`, "foo", "1")
	w := httptest.NewRecorder()
	h.HandleItemTags(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_AttachTag_ItemLookupError(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnError(errors.New("boom"))

	h := NewItemsHandler(db)
	r := itemReq("POST", "/v1/projects/foo/items/1/tags",
		`{"tag_slug":"bug"}`, "foo", "1")
	w := httptest.NewRecorder()
	h.HandleItemTags(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_AttachTag_TagNotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnRows(itemRows())
	mock.ExpectQuery(`FROM tags WHERE user_id = \$1 AND slug = \$2`).
		WithArgs(testUserID, "missing").
		WillReturnError(sql.ErrNoRows)

	h := NewItemsHandler(db)
	r := itemReq("POST", "/v1/projects/foo/items/1/tags",
		`{"tag_slug":"missing"}`, "foo", "1")
	w := httptest.NewRecorder()
	h.HandleItemTags(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_AttachTag_TagLookupError(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnRows(itemRows())
	mock.ExpectQuery(`FROM tags WHERE user_id = \$1 AND slug = \$2`).
		WithArgs(testUserID, "bug").
		WillReturnError(errors.New("boom"))

	h := NewItemsHandler(db)
	r := itemReq("POST", "/v1/projects/foo/items/1/tags",
		`{"tag_slug":"bug"}`, "foo", "1")
	w := httptest.NewRecorder()
	h.HandleItemTags(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_AttachTag_InsertError(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnRows(itemRows())
	mock.ExpectQuery(`FROM tags WHERE user_id = \$1 AND slug = \$2`).
		WithArgs(testUserID, "bug").
		WillReturnRows(tagRows())
	mock.ExpectExec(`INSERT INTO item_tags`).
		WithArgs(testItemID, testTagID, testUserID).
		WillReturnError(errors.New("boom"))

	h := NewItemsHandler(db)
	r := itemReq("POST", "/v1/projects/foo/items/1/tags",
		`{"tag_slug":"bug"}`, "foo", "1")
	w := httptest.NewRecorder()
	h.HandleItemTags(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_AttachTag_Forbidden(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, "other-owner")

	h := NewItemsHandler(db)
	r := itemReq("POST", "/v1/projects/foo/items/1/tags",
		`{"tag_slug":"bug"}`, "foo", "1")
	w := httptest.NewRecorder()
	h.HandleItemTags(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("got %d", w.Code)
	}
}

// HandleItemTagByID

func TestItems_DetachTag_Success(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnRows(itemRows())
	mock.ExpectQuery(`FROM tags WHERE user_id = \$1 AND slug = \$2`).
		WithArgs(testUserID, "bug").
		WillReturnRows(tagRows())
	mock.ExpectExec(`DELETE FROM item_tags`).
		WithArgs(testItemID, testTagID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	h := NewItemsHandler(db)
	r := itemReq("DELETE", "/v1/projects/foo/items/1/tags/bug", "", "foo", "1")
	r.SetPathValue("tagSlug", "bug")
	w := httptest.NewRecorder()
	h.HandleItemTagByID(w, r)
	if w.Code != http.StatusNoContent {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_DetachTag_MissingArgs(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewItemsHandler(db)
	r := authedReq("DELETE", "/v1/projects/foo/items/1/tags/bug", "")
	w := httptest.NewRecorder()
	h.HandleItemTagByID(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_DetachTag_BadSeq(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewItemsHandler(db)
	r := itemReq("DELETE", "/v1/projects/foo/items/abc/tags/bug", "", "foo", "abc")
	r.SetPathValue("tagSlug", "bug")
	w := httptest.NewRecorder()
	h.HandleItemTagByID(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_DetachTag_MethodNotAllowed(t *testing.T) {
	db, _ := newMockDB(t)
	h := NewItemsHandler(db)
	r := itemReq("POST", "/v1/projects/foo/items/1/tags/bug", "", "foo", "1")
	r.SetPathValue("tagSlug", "bug")
	w := httptest.NewRecorder()
	h.HandleItemTagByID(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_DetachTag_ItemNotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnError(sql.ErrNoRows)

	h := NewItemsHandler(db)
	r := itemReq("DELETE", "/v1/projects/foo/items/1/tags/bug", "", "foo", "1")
	r.SetPathValue("tagSlug", "bug")
	w := httptest.NewRecorder()
	h.HandleItemTagByID(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_DetachTag_ItemLookupError(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnError(errors.New("boom"))

	h := NewItemsHandler(db)
	r := itemReq("DELETE", "/v1/projects/foo/items/1/tags/bug", "", "foo", "1")
	r.SetPathValue("tagSlug", "bug")
	w := httptest.NewRecorder()
	h.HandleItemTagByID(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_DetachTag_TagNotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnRows(itemRows())
	mock.ExpectQuery(`FROM tags WHERE user_id = \$1 AND slug = \$2`).
		WithArgs(testUserID, "missing").
		WillReturnError(sql.ErrNoRows)

	h := NewItemsHandler(db)
	r := itemReq("DELETE", "/v1/projects/foo/items/1/tags/missing", "", "foo", "1")
	r.SetPathValue("tagSlug", "missing")
	w := httptest.NewRecorder()
	h.HandleItemTagByID(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_DetachTag_TagLookupError(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnRows(itemRows())
	mock.ExpectQuery(`FROM tags WHERE user_id = \$1 AND slug = \$2`).
		WithArgs(testUserID, "bug").
		WillReturnError(errors.New("boom"))

	h := NewItemsHandler(db)
	r := itemReq("DELETE", "/v1/projects/foo/items/1/tags/bug", "", "foo", "1")
	r.SetPathValue("tagSlug", "bug")
	w := httptest.NewRecorder()
	h.HandleItemTagByID(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_DetachTag_DeleteError(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, testUserID)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs(testProjectID, 1).
		WillReturnRows(itemRows())
	mock.ExpectQuery(`FROM tags WHERE user_id = \$1 AND slug = \$2`).
		WithArgs(testUserID, "bug").
		WillReturnRows(tagRows())
	mock.ExpectExec(`DELETE FROM item_tags`).
		WithArgs(testItemID, testTagID).
		WillReturnError(errors.New("boom"))

	h := NewItemsHandler(db)
	r := itemReq("DELETE", "/v1/projects/foo/items/1/tags/bug", "", "foo", "1")
	r.SetPathValue("tagSlug", "bug")
	w := httptest.NewRecorder()
	h.HandleItemTagByID(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d", w.Code)
	}
}

func TestItems_DetachTag_Forbidden(t *testing.T) {
	db, mock := newMockDB(t)
	mockProjectLookup(t, mock, "other-owner")

	h := NewItemsHandler(db)
	r := itemReq("DELETE", "/v1/projects/foo/items/1/tags/bug", "", "foo", "1")
	r.SetPathValue("tagSlug", "bug")
	w := httptest.NewRecorder()
	h.HandleItemTagByID(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("got %d", w.Code)
	}
}
