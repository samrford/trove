package data

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

func TestIsValidItemKind(t *testing.T) {
	for _, k := range AllItemKinds {
		if !IsValidItemKind(string(k)) {
			t.Errorf("%q should be valid", k)
		}
	}
	for _, k := range []string{"", "other", "TASK"} {
		if IsValidItemKind(k) {
			t.Errorf("%q should be invalid", k)
		}
	}
}

func TestIsValidItemStatus(t *testing.T) {
	for _, s := range AllItemStatuses {
		if !IsValidItemStatus(string(s)) {
			t.Errorf("%q should be valid", s)
		}
	}
	for _, s := range []string{"", "Open", "pending"} {
		if IsValidItemStatus(s) {
			t.Errorf("%q should be invalid", s)
		}
	}
}

func itemRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "project_id", "sequence", "kind", "status", "title", "body",
		"position", "creator_id", "created_at", "updated_at",
	}).AddRow("i-1", "p-1", 1, string(ItemKindTask), string(ItemStatusOpen), "Title", nil, 1.0, "u-1", time.Now(), time.Now())
}

func TestCreateItem_Success(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`UPDATE project_item_sequences\s+SET next_sequence = next_sequence \+ 1`).
		WithArgs("p-1").
		WillReturnRows(sqlmock.NewRows([]string{"seq"}).AddRow(1))
	mock.ExpectQuery(`INSERT INTO items`).
		WithArgs("p-1", 1, ItemKindTask, "Title", sql.NullString{}, "u-1").
		WillReturnRows(itemRows())
	mock.ExpectCommit()

	item, err := CreateItem(context.Background(), db, "p-1", "u-1", ItemKindTask, "Title", nil)
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	if item.ID != "i-1" || item.Sequence != 1 {
		t.Errorf("got %+v", item)
	}
}

func TestCreateItem_SequenceError(t *testing.T) {
	db, mock := newMock(t)
	seqErr := errors.New("seq")
	mock.ExpectBegin()
	mock.ExpectQuery(`UPDATE project_item_sequences`).
		WithArgs("p-1").
		WillReturnError(seqErr)
	mock.ExpectRollback()

	_, err := CreateItem(context.Background(), db, "p-1", "u-1", ItemKindTask, "T", nil)
	if !errors.Is(err, seqErr) {
		t.Fatalf("expected wrapped sequence error, got %v", err)
	}
}

func TestCreateItem_InsertError(t *testing.T) {
	db, mock := newMock(t)
	insErr := errors.New("boom")
	mock.ExpectBegin()
	mock.ExpectQuery(`UPDATE project_item_sequences`).
		WithArgs("p-1").
		WillReturnRows(sqlmock.NewRows([]string{"seq"}).AddRow(1))
	mock.ExpectQuery(`INSERT INTO items`).
		WithArgs("p-1", 1, ItemKindTask, "T", sql.NullString{}, "u-1").
		WillReturnError(insErr)
	mock.ExpectRollback()

	_, err := CreateItem(context.Background(), db, "p-1", "u-1", ItemKindTask, "T", nil)
	if !errors.Is(err, insErr) {
		t.Fatalf("expected the insert error to propagate, got %v", err)
	}
}

func TestCreateItem_BeginError(t *testing.T) {
	db, mock := newMock(t)
	beginErr := errors.New("nope")
	mock.ExpectBegin().WillReturnError(beginErr)

	_, err := CreateItem(context.Background(), db, "p-1", "u-1", ItemKindTask, "T", nil)
	if !errors.Is(err, beginErr) {
		t.Fatalf("expected the begin error to propagate, got %v", err)
	}
}

func TestGetItemBySequence_Success(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`SELECT .* FROM items WHERE project_id = \$1 AND sequence = \$2`).
		WithArgs("p-1", 5).
		WillReturnRows(itemRows())

	it, err := GetItemBySequence(context.Background(), db, "p-1", 5)
	if err != nil {
		t.Fatalf("GetItemBySequence: %v", err)
	}
	if it.ID != "i-1" {
		t.Errorf("got %+v", it)
	}
}

func TestGetItemBySequence_NotFound(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`FROM items WHERE project_id`).
		WithArgs("p-1", 5).
		WillReturnError(sql.ErrNoRows)

	if _, err := GetItemBySequence(context.Background(), db, "p-1", 5); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected ErrNoRows, got %v", err)
	}
}

func TestListItemsForProject_NoFilters(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`FROM items i\s+WHERE i\.project_id = \$1`).
		WithArgs("p-1").
		WillReturnRows(itemRows())

	got, err := ListItemsForProject(context.Background(), db, "p-1", ItemFilter{})
	if err != nil {
		t.Fatalf("ListItemsForProject: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("want 1, got %d", len(got))
	}
}

func TestListItemsForProject_KindAndStatus(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`i\.project_id = \$1 AND i\.kind = \$2 AND i\.status = \$3`).
		WithArgs("p-1", string(ItemKindTask), string(ItemStatusOpen)).
		WillReturnRows(itemRows())

	if _, err := ListItemsForProject(context.Background(), db, "p-1", ItemFilter{
		Kind:   string(ItemKindTask),
		Status: string(ItemStatusOpen),
	}); err != nil {
		t.Fatalf("ListItemsForProject: %v", err)
	}
}

// The AND/OR tag filters generate structurally different SQL. These regexes
// pin each branch's full sub-select (including the cardinality / EXISTS shape)
// so a regression that swaps the branches — or degrades AND into OR — fails
// the match rather than passing on a loose `FROM items i`.
const (
	tagFilterAndSQL = `i\.project_id = \$1 AND \(\s+SELECT COUNT\(DISTINCT t\.slug\) FROM tags t\s+JOIN item_tags it ON it\.tag_id = t\.id\s+WHERE it\.item_id = i\.id AND t\.slug = ANY\(\$2\)\s+\) = cardinality\(\$2\)`
	tagFilterOrSQL  = `i\.project_id = \$1 AND EXISTS \(\s+SELECT 1 FROM item_tags it\s+JOIN tags t ON t\.id = it\.tag_id\s+WHERE it\.item_id = i\.id AND t\.slug = ANY\(\$2\)\s+\)`
)

func TestListItemsForProject_TagFilterAnd(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(tagFilterAndSQL).
		WithArgs("p-1", pq.Array([]string{"bug", "urgent"})).
		WillReturnRows(itemRows())

	if _, err := ListItemsForProject(context.Background(), db, "p-1", ItemFilter{
		TagSlugs: []string{"bug", "urgent"},
	}); err != nil {
		t.Fatalf("ListItemsForProject: %v", err)
	}
}

func TestListItemsForProject_TagFilterOr(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(tagFilterOrSQL).
		WithArgs("p-1", pq.Array([]string{"bug"})).
		WillReturnRows(itemRows())

	if _, err := ListItemsForProject(context.Background(), db, "p-1", ItemFilter{
		TagSlugs: []string{"bug"}, TagMode: "or",
	}); err != nil {
		t.Fatalf("ListItemsForProject: %v", err)
	}
}

func TestListItemsForProject_QueryError(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`FROM items i`).
		WithArgs("p-1").
		WillReturnError(errors.New("boom"))

	if _, err := ListItemsForProject(context.Background(), db, "p-1", ItemFilter{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestListItemsForProject_ScanError(t *testing.T) {
	db, mock := newMock(t)
	rows := sqlmock.NewRows([]string{"id"}).AddRow("i-1")
	mock.ExpectQuery(`FROM items i`).WithArgs("p-1").WillReturnRows(rows)

	if _, err := ListItemsForProject(context.Background(), db, "p-1", ItemFilter{}); err == nil {
		t.Fatal("expected scan error")
	}
}

func TestUpdateItem_FullPatch(t *testing.T) {
	db, mock := newMock(t)
	title := "New"
	body := strPtr("body")
	kind := ItemKindTask
	status := ItemStatusDone
	pos := 2.5
	patch := ItemPatch{
		Title:    &title,
		Body:     &body,
		Kind:     &kind,
		Status:   &status,
		Position: &pos,
	}
	mock.ExpectQuery(`UPDATE items SET updated_at = NOW\(\), title = \$1, body = \$2, kind = \$3, status = \$4, position = \$5 WHERE id = \$6`).
		WithArgs("New", "body", kind, status, pos, "i-1").
		WillReturnRows(itemRows())

	if _, err := UpdateItem(context.Background(), db, "i-1", patch); err != nil {
		t.Fatalf("UpdateItem: %v", err)
	}
}

func TestUpdateItem_ClearBody(t *testing.T) {
	db, mock := newMock(t)
	// Body is **string; patch.Body != nil but *patch.Body (inner *string) == nil
	// clears the column.
	var inner *string
	patch := ItemPatch{Body: &inner}
	mock.ExpectQuery(`UPDATE items SET updated_at = NOW\(\), body = \$1 WHERE id = \$2`).
		WithArgs(sql.NullString{}, "i-1").
		WillReturnRows(itemRows())

	if _, err := UpdateItem(context.Background(), db, "i-1", patch); err != nil {
		t.Fatalf("UpdateItem: %v", err)
	}
}

func TestUpdateItem_EmptyPatch(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`UPDATE items SET updated_at = NOW\(\) WHERE id = \$1`).
		WithArgs("i-1").
		WillReturnRows(itemRows())

	if _, err := UpdateItem(context.Background(), db, "i-1", ItemPatch{}); err != nil {
		t.Fatalf("UpdateItem: %v", err)
	}
}

func TestDeleteItem_Success(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectExec(`DELETE FROM items WHERE id = \$1`).
		WithArgs("i-1").
		WillReturnResult(sqlmockResult(1))

	if err := DeleteItem(context.Background(), db, "i-1"); err != nil {
		t.Fatalf("DeleteItem: %v", err)
	}
}

func TestDeleteItem_NotFound(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectExec(`DELETE FROM items`).
		WithArgs("i-1").
		WillReturnResult(sqlmockResult(0))

	if err := DeleteItem(context.Background(), db, "i-1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected ErrNoRows, got %v", err)
	}
}

func TestDeleteItem_ExecError(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectExec(`DELETE FROM items`).
		WithArgs("i-1").
		WillReturnError(errors.New("boom"))

	if err := DeleteItem(context.Background(), db, "i-1"); err == nil {
		t.Fatal("expected error")
	}
}
