package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCorsMiddleware_OptionsShortCircuits(t *testing.T) {
	called := false
	h := corsMiddleware(func(http.ResponseWriter, *http.Request) { called = true })

	r := httptest.NewRequest("OPTIONS", "/v1/health", nil)
	w := httptest.NewRecorder()
	h(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("got %d", w.Code)
	}
	if called {
		t.Error("next should not be called for OPTIONS")
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("got %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("expected methods header set")
	}
	if got := w.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Error("expected headers header set")
	}
}

func TestCorsMiddleware_PassesThrough(t *testing.T) {
	called := false
	h := corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	})

	r := httptest.NewRequest("GET", "/v1/health", nil)
	w := httptest.NewRecorder()
	h(w, r)

	if !called {
		t.Error("next not called")
	}
	if w.Code != http.StatusTeapot {
		t.Errorf("got %d", w.Code)
	}
}
