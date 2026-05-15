package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/coreos/go-oidc/v3/oidc"
)

func TestGetUserID_Empty(t *testing.T) {
	if got := GetUserID(context.Background()); got != "" {
		t.Errorf("want empty, got %q", got)
	}
}

func TestGetUserEmail_Empty(t *testing.T) {
	if got := GetUserEmail(context.Background()); got != "" {
		t.Errorf("want empty, got %q", got)
	}
}

func TestGetUserID_Set(t *testing.T) {
	ctx := context.WithValue(context.Background(), userIDKey, "abc")
	if got := GetUserID(ctx); got != "abc" {
		t.Errorf("want abc, got %q", got)
	}
}

func TestGetUserEmail_Set(t *testing.T) {
	ctx := context.WithValue(context.Background(), userEmailKey, "u@x.com")
	if got := GetUserEmail(ctx); got != "u@x.com" {
		t.Errorf("want u@x.com, got %q", got)
	}
}

func TestAuthMiddleware_MissingHeader(t *testing.T) {
	db, _ := newMockDB(t)
	called := false
	h := AuthMiddleware(&fakeVerifier{}, db, func(w http.ResponseWriter, r *http.Request) { called = true })
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest("GET", "/v1/me", nil))
	if called {
		t.Fatal("next called")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_WrongScheme(t *testing.T) {
	db, _ := newMockDB(t)
	h := AuthMiddleware(&fakeVerifier{}, db, func(w http.ResponseWriter, r *http.Request) {})
	r := httptest.NewRequest("GET", "/v1/me", nil)
	r.Header.Set("Authorization", "Basic abc")
	w := httptest.NewRecorder()
	h(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	db, _ := newMockDB(t)
	h := AuthMiddleware(&fakeVerifier{err: errors.New("bad")}, db, func(w http.ResponseWriter, r *http.Request) {})
	r := httptest.NewRequest("GET", "/v1/me", nil)
	r.Header.Set("Authorization", "Bearer xxx")
	w := httptest.NewRecorder()
	h(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_EmptySubject(t *testing.T) {
	db, _ := newMockDB(t)
	h := AuthMiddleware(&fakeVerifier{token: &oidc.IDToken{Subject: ""}}, db, func(w http.ResponseWriter, r *http.Request) {})
	r := httptest.NewRequest("GET", "/v1/me", nil)
	r.Header.Set("Authorization", "Bearer xxx")
	w := httptest.NewRecorder()
	h(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_UpsertError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec(`INSERT INTO users`).
		WithArgs("u-1", "").
		WillReturnError(errors.New("boom"))

	h := AuthMiddleware(&fakeVerifier{token: &oidc.IDToken{Subject: "u-1"}}, db, func(w http.ResponseWriter, r *http.Request) {})
	r := httptest.NewRequest("GET", "/v1/me", nil)
	r.Header.Set("Authorization", "Bearer xxx")
	w := httptest.NewRecorder()
	h(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", w.Code)
	}
}

func TestAuthMiddleware_Success(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec(`INSERT INTO users`).
		WithArgs("u-1", "").
		WillReturnResult(sqlmock.NewResult(0, 1))

	var gotID string
	h := AuthMiddleware(&fakeVerifier{token: &oidc.IDToken{Subject: "u-1"}}, db, func(w http.ResponseWriter, r *http.Request) {
		gotID = GetUserID(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	r := httptest.NewRequest("GET", "/v1/me", nil)
	r.Header.Set("Authorization", "Bearer xxx")
	w := httptest.NewRecorder()
	h(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
	if gotID != "u-1" {
		t.Errorf("want u-1, got %q", gotID)
	}
}

// InitAuth requires a live OIDC issuer to perform discovery; we exercise the
// error path (unreachable issuer) so the function still gets coverage.
func TestInitAuth_UnreachableIssuer(t *testing.T) {
	if _, err := InitAuth(context.Background(), "http://127.0.0.1:1/"); err == nil {
		t.Fatal("expected error contacting bogus issuer")
	}
}
