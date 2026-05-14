package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleMe_Success(t *testing.T) {
	r := authedReq("GET", "/v1/me", "")
	w := httptest.NewRecorder()
	HandleMe(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
	var got map[string]string
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["id"] != testUserID || got["email"] != testUserEmail {
		t.Errorf("got %+v", got)
	}
}

func TestHandleMe_WrongMethod(t *testing.T) {
	r := authedReq("POST", "/v1/me", "")
	w := httptest.NewRecorder()
	HandleMe(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", w.Code)
	}
}
