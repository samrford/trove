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

func TestNormaliseTagName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Bug", "bug"},
		{"  BUG  ", "bug"},
		{"Multi Word", "multi word"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := normaliseTagName(tc.in); got != tc.want {
			t.Errorf("normaliseTagName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func tagRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "slug", "name", "name_normalised", "description", "icon", "colour",
		"user_id", "group_id", "archived_at", "created_at", "updated_at",
	}).AddRow("t-1", "bug", "Bug", "bug", nil, nil, nil, "u-1", nil, nil, time.Now(), time.Now())
}

func TestTagSlugExistsForOwner(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM tags WHERE user_id = \$1 AND slug = \$2\)`).
		WithArgs("u-1", "bug").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	exists, err := TagSlugExistsForOwner(context.Background(), db, "u-1", "bug")
	if err != nil || exists {
		t.Fatalf("got exists=%v err=%v", exists, err)
	}
}

func TestFindOrCreateTag_Success(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`INSERT INTO tags`).
		WithArgs("bug", "Bug", "bug", sql.NullString{}, sql.NullString{}, sql.NullString{}, "u-1").
		WillReturnRows(tagRows())

	tag, created, err := FindOrCreateTag(context.Background(), db, "u-1", "Bug", "bug", nil, nil, nil)
	if err != nil {
		t.Fatalf("FindOrCreateTag: %v", err)
	}
	if !created || tag.ID != "t-1" {
		t.Errorf("created=%v tag=%+v", created, tag)
	}
}

func TestFindOrCreateTag_AutoSlug(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`INSERT INTO tags`).
		WithArgs("bug-bash", "Bug Bash", "bug bash", sql.NullString{}, sql.NullString{}, sql.NullString{}, "u-1").
		WillReturnRows(tagRows())

	if _, _, err := FindOrCreateTag(context.Background(), db, "u-1", "Bug Bash", "", nil, nil, nil); err != nil {
		t.Fatalf("FindOrCreateTag: %v", err)
	}
}

func TestFindOrCreateTag_EmptyName(t *testing.T) {
	db, _ := newMock(t)
	if _, _, err := FindOrCreateTag(context.Background(), db, "u-1", "  ", "", nil, nil, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestFindOrCreateTag_UnslugifiableName(t *testing.T) {
	db, _ := newMock(t)
	if _, _, err := FindOrCreateTag(context.Background(), db, "u-1", "!!!", "", nil, nil, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestFindOrCreateTag_AutoMerge(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`INSERT INTO tags`).
		WithArgs("bug", "Bug", "bug", sql.NullString{}, sql.NullString{}, sql.NullString{}, "u-1").
		WillReturnError(&pq.Error{Code: "23505", Constraint: "tags_owner_normalised_unique"})
	mock.ExpectQuery(`SELECT .* FROM tags WHERE user_id = \$1 AND name_normalised = \$2`).
		WithArgs("u-1", "bug").
		WillReturnRows(tagRows())

	tag, created, err := FindOrCreateTag(context.Background(), db, "u-1", "Bug", "bug", nil, nil, nil)
	if err != nil {
		t.Fatalf("FindOrCreateTag: %v", err)
	}
	if created || tag.ID != "t-1" {
		t.Errorf("created=%v tag=%+v", created, tag)
	}
}

func TestFindOrCreateTag_SlugTaken(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`INSERT INTO tags`).
		WithArgs("bug", "Bug", "bug", sql.NullString{}, sql.NullString{}, sql.NullString{}, "u-1").
		WillReturnError(&pq.Error{Code: "23505", Constraint: "tags_owner_slug_unique"})

	if _, _, err := FindOrCreateTag(context.Background(), db, "u-1", "Bug", "bug", nil, nil, nil); !errors.Is(err, ErrTagSlugTaken) {
		t.Fatalf("expected ErrTagSlugTaken, got %v", err)
	}
}

func TestFindOrCreateTag_UnknownPQError(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`INSERT INTO tags`).
		WithArgs("bug", "Bug", "bug", sql.NullString{}, sql.NullString{}, sql.NullString{}, "u-1").
		WillReturnError(&pq.Error{Code: "23505", Constraint: "something_else"})

	if _, _, err := FindOrCreateTag(context.Background(), db, "u-1", "Bug", "bug", nil, nil, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestFindOrCreateTag_OtherError(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`INSERT INTO tags`).
		WithArgs("bug", "Bug", "bug", sql.NullString{}, sql.NullString{}, sql.NullString{}, "u-1").
		WillReturnError(errors.New("boom"))

	if _, _, err := FindOrCreateTag(context.Background(), db, "u-1", "Bug", "bug", nil, nil, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestGetTagForUser_BySlug(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`FROM tags WHERE user_id = \$1 AND slug = \$2`).
		WithArgs("u-1", "bug").
		WillReturnRows(tagRows())

	if _, err := GetTagForUser(context.Background(), db, "u-1", "bug"); err != nil {
		t.Fatalf("GetTagForUser: %v", err)
	}
}

func TestGetTagForUser_ByUUID(t *testing.T) {
	db, mock := newMock(t)
	id := "11111111-1111-1111-1111-111111111111"
	mock.ExpectQuery(`FROM tags WHERE user_id = \$1 AND id = \$2`).
		WithArgs("u-1", id).
		WillReturnRows(tagRows())

	if _, err := GetTagForUser(context.Background(), db, "u-1", id); err != nil {
		t.Fatalf("GetTagForUser: %v", err)
	}
}

func TestListTagsForUser(t *testing.T) {
	db, mock := newMock(t)
	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"id", "slug", "name", "name_normalised", "description", "icon", "colour",
		"user_id", "group_id", "archived_at", "created_at", "updated_at",
		"item_count", "last_used_at",
	}).AddRow("t-1", "bug", "Bug", "bug", nil, nil, nil, "u-1", nil, nil, now, now, 3, now).
		AddRow("t-2", "ui", "UI", "ui", nil, nil, nil, "u-1", nil, nil, now, now, 0, nil)
	mock.ExpectQuery(`FROM tags t\s+LEFT JOIN \(\s+SELECT tag_id, COUNT`).
		WithArgs("u-1").
		WillReturnRows(rows)

	got, err := ListTagsForUser(context.Background(), db, "u-1")
	if err != nil {
		t.Fatalf("ListTagsForUser: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2, got %d", len(got))
	}
}

func TestListTagsForUser_QueryError(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`FROM tags t`).
		WithArgs("u-1").
		WillReturnError(errors.New("boom"))

	if _, err := ListTagsForUser(context.Background(), db, "u-1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestListTagsForUser_ScanError(t *testing.T) {
	db, mock := newMock(t)
	rows := sqlmock.NewRows([]string{"id"}).AddRow("t-1")
	mock.ExpectQuery(`FROM tags t`).WithArgs("u-1").WillReturnRows(rows)

	if _, err := ListTagsForUser(context.Background(), db, "u-1"); err == nil {
		t.Fatal("expected scan error")
	}
}

func TestUpdateTag_Success(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`UPDATE tags`).
		WithArgs("Bug", "bug", "bug", sql.NullString{}, sql.NullString{}, sql.NullString{}, "t-1").
		WillReturnRows(tagRows())

	if _, err := UpdateTag(context.Background(), db, "t-1", "Bug", "bug", nil, nil, nil); err != nil {
		t.Fatalf("UpdateTag: %v", err)
	}
}

func TestUpdateTag_EmptyName(t *testing.T) {
	db, _ := newMock(t)
	if _, err := UpdateTag(context.Background(), db, "t-1", "  ", "", nil, nil, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateTag_SlugTaken(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`UPDATE tags`).
		WithArgs("Bug", "bug", "bug", sql.NullString{}, sql.NullString{}, sql.NullString{}, "t-1").
		WillReturnError(&pq.Error{Code: "23505", Constraint: "tags_owner_slug_unique"})

	if _, err := UpdateTag(context.Background(), db, "t-1", "Bug", "bug", nil, nil, nil); !errors.Is(err, ErrTagSlugTaken) {
		t.Fatalf("expected ErrTagSlugTaken, got %v", err)
	}
}

func TestUpdateTag_NormalisedConflict(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`UPDATE tags`).
		WithArgs("Bug", "bug", "bug", sql.NullString{}, sql.NullString{}, sql.NullString{}, "t-1").
		WillReturnError(&pq.Error{Code: "23505", Constraint: "tags_owner_normalised_unique"})

	if _, err := UpdateTag(context.Background(), db, "t-1", "Bug", "bug", nil, nil, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateTag_OtherError(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`UPDATE tags`).
		WithArgs("Bug", "bug", "bug", sql.NullString{}, sql.NullString{}, sql.NullString{}, "t-1").
		WillReturnError(errors.New("boom"))

	if _, err := UpdateTag(context.Background(), db, "t-1", "Bug", "bug", nil, nil, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestDeleteTag_Success(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectExec(`DELETE FROM tags WHERE id = \$1`).
		WithArgs("t-1").
		WillReturnResult(sqlmockResult(1))

	if err := DeleteTag(context.Background(), db, "t-1"); err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}
}

func TestDeleteTag_NotFound(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectExec(`DELETE FROM tags`).
		WithArgs("t-1").
		WillReturnResult(sqlmockResult(0))

	if err := DeleteTag(context.Background(), db, "t-1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected ErrNoRows, got %v", err)
	}
}

func TestDeleteTag_ExecError(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectExec(`DELETE FROM tags`).
		WithArgs("t-1").
		WillReturnError(errors.New("boom"))

	if err := DeleteTag(context.Background(), db, "t-1"); err == nil {
		t.Fatal("expected error")
	}
}
