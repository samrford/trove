package data

import (
	"database/sql"
	"database/sql/driver"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// newMock returns a *sql.DB backed by sqlmock using regexp query matching, so
// tests can write SQL fragments with literal whitespace/newlines without
// having to escape every metacharacter.
func newMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
		db.Close()
	})
	return db, mock
}

func strPtr(s string) *string { return &s }

func sqlmockResult(rowsAffected int64) driver.Result {
	return sqlmock.NewResult(0, rowsAffected)
}
