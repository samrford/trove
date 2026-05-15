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

func TestIsValidSlug(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"valid-slug", true},
		{"a", true},
		{"123", true},
		{"abc-123", true},
		{"-leading", false},
		{"trailing-", false},
		{"double--dash", false},
		{"UPPER", false},
		{"with space", false},
		{"sym!", false},
	}
	for _, tc := range cases {
		if got := IsValidSlug(tc.in); got != tc.want {
			t.Errorf("IsValidSlug(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestGenerateSlug(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"My Project!", "my-project"},
		{"  spaced  ", "spaced"},
		{"Foo-Bar", "foo-bar"},
		{"!!!", ""},
		{"trailing!!", "trailing"},
		{"Multi   Spaces", "multi-spaces"},
		{"weird___chars", "weird-chars"},
		{"already-slug", "already-slug"},
	}
	for _, tc := range cases {
		if got := generateSlug(tc.in); got != tc.want {
			t.Errorf("generateSlug(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSlugExistsForOwner(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM projects WHERE owner_id = \$1 AND slug = \$2\)`).
		WithArgs("u-1", "foo").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	ok, err := SlugExistsForOwner(context.Background(), db, "u-1", "foo")
	if err != nil || !ok {
		t.Fatalf("SlugExistsForOwner: ok=%v err=%v", ok, err)
	}
}

func TestSlugExistsForOwner_Error(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs("u-1", "foo").
		WillReturnError(errors.New("db down"))

	if _, err := SlugExistsForOwner(context.Background(), db, "u-1", "foo"); err == nil {
		t.Fatal("expected error")
	}
}

func projectRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "slug", "name", "description", "colour", "icon",
		"owner_id", "archived_at", "created_at", "updated_at",
	}).AddRow("p-1", "foo", "Foo", nil, nil, nil, "u-1", nil, time.Now(), time.Now())
}

func TestCreateProject_Success(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO projects`).
		WithArgs("foo", "Foo", sql.NullString{}, sql.NullString{}, sql.NullString{}, "u-1").
		WillReturnRows(projectRows())
	mock.ExpectExec(`INSERT INTO project_item_sequences`).
		WithArgs("p-1").
		WillReturnResult(sqlmockResult(1))
	mock.ExpectCommit()

	p, err := CreateProject(context.Background(), db, "u-1", "foo", "Foo", nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if p.ID != "p-1" || p.Slug != "foo" {
		t.Errorf("unexpected: %+v", p)
	}
}

func TestCreateProject_AutoSlug(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO projects`).
		WithArgs("my-project", "My Project", sql.NullString{}, sql.NullString{}, sql.NullString{}, "u-1").
		WillReturnRows(projectRows())
	mock.ExpectExec(`INSERT INTO project_item_sequences`).
		WithArgs("p-1").
		WillReturnResult(sqlmockResult(1))
	mock.ExpectCommit()

	if _, err := CreateProject(context.Background(), db, "u-1", "", "My Project", nil, nil, nil); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
}

func TestCreateProject_EmptyNameAfterSlugify(t *testing.T) {
	db, _ := newMock(t)
	_, err := CreateProject(context.Background(), db, "u-1", "", "!!!", nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for unslugifiable name")
	}
}

func TestCreateProject_SlugTaken(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO projects`).
		WithArgs("foo", "Foo", sql.NullString{}, sql.NullString{}, sql.NullString{}, "u-1").
		WillReturnError(&pq.Error{Code: PGUniqueViolation})
	mock.ExpectRollback()

	_, err := CreateProject(context.Background(), db, "u-1", "foo", "Foo", nil, nil, nil)
	if !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("expected ErrSlugTaken, got %v", err)
	}
}

func TestCreateProject_InsertOtherError(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO projects`).
		WithArgs("foo", "Foo", sql.NullString{}, sql.NullString{}, sql.NullString{}, "u-1").
		WillReturnError(errors.New("boom"))
	mock.ExpectRollback()

	if _, err := CreateProject(context.Background(), db, "u-1", "foo", "Foo", nil, nil, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateProject_SequenceError(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO projects`).
		WithArgs("foo", "Foo", sql.NullString{}, sql.NullString{}, sql.NullString{}, "u-1").
		WillReturnRows(projectRows())
	mock.ExpectExec(`INSERT INTO project_item_sequences`).
		WithArgs("p-1").
		WillReturnError(errors.New("seq error"))
	mock.ExpectRollback()

	if _, err := CreateProject(context.Background(), db, "u-1", "foo", "Foo", nil, nil, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateProject_BeginError(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectBegin().WillReturnError(errors.New("nope"))

	if _, err := CreateProject(context.Background(), db, "u-1", "foo", "Foo", nil, nil, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestListProjectsForUser(t *testing.T) {
	db, mock := newMock(t)
	rows := projectRows().AddRow("p-2", "bar", "Bar", strPtr("d"), strPtr("#fff"), strPtr("icon"), "u-1", nil, time.Now(), time.Now())
	mock.ExpectQuery(`FROM projects p\s+LEFT JOIN project_members`).
		WithArgs("u-1").
		WillReturnRows(rows)

	got, err := ListProjectsForUser(context.Background(), db, "u-1")
	if err != nil {
		t.Fatalf("ListProjectsForUser: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 projects, got %d", len(got))
	}
}

func TestListProjectsForUser_QueryError(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`FROM projects p`).
		WithArgs("u-1").
		WillReturnError(errors.New("boom"))

	if _, err := ListProjectsForUser(context.Background(), db, "u-1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestListProjectsForUser_ScanError(t *testing.T) {
	db, mock := newMock(t)
	rows := sqlmock.NewRows([]string{"id"}).AddRow("p-1") // wrong column count
	mock.ExpectQuery(`FROM projects p`).WithArgs("u-1").WillReturnRows(rows)

	if _, err := ListProjectsForUser(context.Background(), db, "u-1"); err == nil {
		t.Fatal("expected scan error")
	}
}

func TestUpdateProject_Success(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`UPDATE projects`).
		WithArgs("Foo", "foo", sql.NullString{}, sql.NullString{}, sql.NullString{}, "p-1").
		WillReturnRows(projectRows())

	p, err := UpdateProject(context.Background(), db, "p-1", "Foo", "foo", nil, nil, nil)
	if err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	if p.ID != "p-1" {
		t.Errorf("got %+v", p)
	}
}

func TestUpdateProject_SlugTaken(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`UPDATE projects`).
		WithArgs("Foo", "foo", sql.NullString{}, sql.NullString{}, sql.NullString{}, "p-1").
		WillReturnError(&pq.Error{Code: PGUniqueViolation})

	if _, err := UpdateProject(context.Background(), db, "p-1", "Foo", "foo", nil, nil, nil); !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("expected ErrSlugTaken, got %v", err)
	}
}

func TestUpdateProject_OtherError(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`UPDATE projects`).
		WithArgs("Foo", "foo", sql.NullString{}, sql.NullString{}, sql.NullString{}, "p-1").
		WillReturnError(errors.New("boom"))

	if _, err := UpdateProject(context.Background(), db, "p-1", "Foo", "foo", nil, nil, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestDeleteProject_Success(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectExec(`DELETE FROM projects WHERE id = \$1`).
		WithArgs("p-1").
		WillReturnResult(sqlmockResult(1))

	if err := DeleteProject(context.Background(), db, "p-1"); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
}

func TestDeleteProject_NotFound(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectExec(`DELETE FROM projects`).
		WithArgs("p-1").
		WillReturnResult(sqlmockResult(0))

	if err := DeleteProject(context.Background(), db, "p-1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected ErrNoRows, got %v", err)
	}
}

func TestDeleteProject_ExecError(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectExec(`DELETE FROM projects`).
		WithArgs("p-1").
		WillReturnError(errors.New("boom"))

	if err := DeleteProject(context.Background(), db, "p-1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestGetProjectForUser_BySlug(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`WHERE p\.slug = \$2`).
		WithArgs("u-1", "foo").
		WillReturnRows(projectRows())

	p, err := GetProjectForUser(context.Background(), db, "u-1", "foo")
	if err != nil {
		t.Fatalf("GetProjectForUser: %v", err)
	}
	if p.ID != "p-1" {
		t.Errorf("got %+v", p)
	}
}

func TestGetProjectForUser_ByUUID(t *testing.T) {
	db, mock := newMock(t)
	id := "11111111-1111-1111-1111-111111111111"
	mock.ExpectQuery(`WHERE p\.id = \$2`).
		WithArgs("u-1", id).
		WillReturnRows(projectRows())

	if _, err := GetProjectForUser(context.Background(), db, "u-1", id); err != nil {
		t.Fatalf("GetProjectForUser: %v", err)
	}
}

func TestGetProjectForUser_NotFound(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`WHERE p\.slug = \$2`).
		WithArgs("u-1", "missing").
		WillReturnError(sql.ErrNoRows)

	if _, err := GetProjectForUser(context.Background(), db, "u-1", "missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected ErrNoRows, got %v", err)
	}
}
