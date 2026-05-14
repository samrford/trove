package data

import (
	"context"
	"errors"
	"testing"
)

func TestUpsertUser_Success(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectExec(`INSERT INTO users \(id, email\)\s+VALUES \(\$1, \$2\)\s+ON CONFLICT \(id\) DO UPDATE SET email = EXCLUDED\.email`).
		WithArgs("u-1", "u@example.com").
		WillReturnResult(sqlmockResult(1))

	if err := UpsertUser(context.Background(), db, "u-1", "u@example.com"); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
}

func TestUpsertUser_Error(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectExec(`INSERT INTO users`).
		WithArgs("u-1", "u@example.com").
		WillReturnError(errors.New("boom"))

	if err := UpsertUser(context.Background(), db, "u-1", "u@example.com"); err == nil {
		t.Fatal("expected error")
	}
}
