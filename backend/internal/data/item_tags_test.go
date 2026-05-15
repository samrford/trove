package data

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

func TestAttachTagToItem(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectExec(`INSERT INTO item_tags`).
		WithArgs("i-1", "t-1", "u-1").
		WillReturnResult(sqlmockResult(1))

	if err := AttachTagToItem(context.Background(), db, "i-1", "t-1", "u-1"); err != nil {
		t.Fatalf("AttachTagToItem: %v", err)
	}
}

func TestAttachTagToItem_Error(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectExec(`INSERT INTO item_tags`).
		WithArgs("i-1", "t-1", "u-1").
		WillReturnError(errors.New("boom"))

	if err := AttachTagToItem(context.Background(), db, "i-1", "t-1", "u-1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestDetachTagFromItem(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectExec(`DELETE FROM item_tags WHERE item_id = \$1 AND tag_id = \$2`).
		WithArgs("i-1", "t-1").
		WillReturnResult(sqlmockResult(1))

	if err := DetachTagFromItem(context.Background(), db, "i-1", "t-1"); err != nil {
		t.Fatalf("DetachTagFromItem: %v", err)
	}
}

func TestGetTagsForItem(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`FROM tags t\s+JOIN item_tags it ON it\.tag_id = t\.id\s+WHERE it\.item_id = \$1`).
		WithArgs("i-1").
		WillReturnRows(tagRows())

	got, err := GetTagsForItem(context.Background(), db, "i-1")
	if err != nil {
		t.Fatalf("GetTagsForItem: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("got %d tags", len(got))
	}
}

func TestGetTagsForItem_QueryError(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`FROM tags t`).
		WithArgs("i-1").
		WillReturnError(errors.New("boom"))

	if _, err := GetTagsForItem(context.Background(), db, "i-1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestGetTagsForItem_ScanError(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`FROM tags t`).
		WithArgs("i-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("t-1"))

	if _, err := GetTagsForItem(context.Background(), db, "i-1"); err == nil {
		t.Fatal("expected scan error")
	}
}

func TestGetTagsForItems_Empty(t *testing.T) {
	db, _ := newMock(t)
	got, err := GetTagsForItems(context.Background(), db, nil)
	if err != nil {
		t.Fatalf("GetTagsForItems: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty map, got %+v", got)
	}
}

func TestGetTagsForItems_Many(t *testing.T) {
	db, mock := newMock(t)
	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"item_id",
		"id", "slug", "name", "name_normalised", "description", "icon", "colour",
		"user_id", "group_id", "archived_at", "created_at", "updated_at",
	}).
		AddRow("i-1", "t-1", "bug", "Bug", "bug", nil, nil, nil, "u-1", nil, nil, now, now).
		AddRow("i-1", "t-2", "ui", "UI", "ui", nil, nil, nil, "u-1", nil, nil, now, now).
		AddRow("i-2", "t-1", "bug", "Bug", "bug", nil, nil, nil, "u-1", nil, nil, now, now)
	mock.ExpectQuery(`WHERE it\.item_id = ANY\(\$1\)`).
		WithArgs(pq.Array([]string{"i-1", "i-2"})).
		WillReturnRows(rows)

	got, err := GetTagsForItems(context.Background(), db, []string{"i-1", "i-2"})
	if err != nil {
		t.Fatalf("GetTagsForItems: %v", err)
	}
	if len(got["i-1"]) != 2 || len(got["i-2"]) != 1 {
		t.Errorf("unexpected: %+v", got)
	}
}

func TestGetTagsForItems_QueryError(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`WHERE it\.item_id = ANY`).
		WithArgs(pq.Array([]string{"i-1"})).
		WillReturnError(errors.New("boom"))

	if _, err := GetTagsForItems(context.Background(), db, []string{"i-1"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestGetTagsForItems_ScanError(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`WHERE it\.item_id = ANY`).
		WithArgs(pq.Array([]string{"i-1"})).
		WillReturnRows(sqlmock.NewRows([]string{"item_id"}).AddRow("i-1"))

	if _, err := GetTagsForItems(context.Background(), db, []string{"i-1"}); err == nil {
		t.Fatal("expected scan error")
	}
}

func TestListTagsUsedInProject(t *testing.T) {
	db, mock := newMock(t)
	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"id", "slug", "name", "name_normalised", "description", "icon", "colour",
		"user_id", "group_id", "archived_at", "created_at", "updated_at",
		"item_count", "last_used_at",
	}).AddRow("t-1", "bug", "Bug", "bug", nil, nil, nil, "u-1", nil, nil, now, now, 5, now)
	mock.ExpectQuery(`JOIN items i\s+ON i\.id = it\.item_id\s+WHERE i\.project_id = \$1`).
		WithArgs("p-1").
		WillReturnRows(rows)

	got, err := ListTagsUsedInProject(context.Background(), db, "p-1")
	if err != nil {
		t.Fatalf("ListTagsUsedInProject: %v", err)
	}
	if len(got) != 1 || got[0].ItemCount != 5 {
		t.Errorf("unexpected: %+v", got)
	}
}

func TestListTagsUsedInProject_QueryError(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`JOIN items i`).
		WithArgs("p-1").
		WillReturnError(errors.New("boom"))

	if _, err := ListTagsUsedInProject(context.Background(), db, "p-1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestListTagsUsedInProject_ScanError(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`JOIN items i`).
		WithArgs("p-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("t-1"))

	if _, err := ListTagsUsedInProject(context.Background(), db, "p-1"); err == nil {
		t.Fatal("expected scan error")
	}
}

func TestListItemsForTag(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`FROM items i\s+JOIN item_tags it ON it\.item_id = i\.id\s+WHERE it\.tag_id = \$1`).
		WithArgs("t-1").
		WillReturnRows(itemRows())

	got, err := ListItemsForTag(context.Background(), db, "t-1")
	if err != nil {
		t.Fatalf("ListItemsForTag: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("got %d", len(got))
	}
}

func TestListItemsForTag_QueryError(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`FROM items i`).
		WithArgs("t-1").
		WillReturnError(errors.New("boom"))

	if _, err := ListItemsForTag(context.Background(), db, "t-1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestListItemsForTag_ScanError(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`FROM items i`).
		WithArgs("t-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("i-1"))

	if _, err := ListItemsForTag(context.Background(), db, "t-1"); err == nil {
		t.Fatal("expected scan error")
	}
}
