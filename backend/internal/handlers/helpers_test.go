package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/coreos/go-oidc/v3/oidc"
)

const (
	testUserID    = "11111111-1111-1111-1111-111111111111"
	testUserEmail = "u@example.com"
	testProjectID = "22222222-2222-2222-2222-222222222222"
	testItemID    = "33333333-3333-3333-3333-333333333333"
	testTagID     = "44444444-4444-4444-4444-444444444444"
)

// newMockDB returns a *sql.DB backed by sqlmock with regexp matching, so
// queries can be matched on substring patterns instead of exact whitespace.
func newMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
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

// decodeJSON unmarshals the recorder body into v, failing the test (with the
// raw body echoed) if it isn't valid JSON of the expected shape. Success-path
// tests use this to assert the serialised contract, not just the status code.
func decodeJSON(t *testing.T, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
		t.Fatalf("decode body %q: %v", w.Body.String(), err)
	}
}

func withUser(r *http.Request, userID, email string) *http.Request {
	ctx := context.WithValue(r.Context(), userIDKey, userID)
	ctx = context.WithValue(ctx, userEmailKey, email)
	return r.WithContext(ctx)
}

func newReq(method, path, body string) *http.Request {
	var b io.Reader = http.NoBody
	if body != "" {
		b = strings.NewReader(body)
	}
	return httptest.NewRequest(method, path, b)
}

func authedReq(method, path, body string) *http.Request {
	return withUser(newReq(method, path, body), testUserID, testUserEmail)
}

// fakeVerifier satisfies IDTokenVerifier with a scripted Verify result.
type fakeVerifier struct {
	token *oidc.IDToken
	err   error
}

func (f *fakeVerifier) Verify(_ context.Context, _ string) (*oidc.IDToken, error) {
	return f.token, f.err
}
