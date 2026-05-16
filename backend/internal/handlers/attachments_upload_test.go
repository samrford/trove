package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"trove/backend/internal/data"
	"trove/backend/internal/data/storage/storagetest"
)

// newUploadTestEnv brings up a real DB, runs Trove's migrations, and seeds an owned
// project+item so the upload handler's resolveItem (GetProjectForUser +
// requireOwner + GetItemBySequence) passes — letting the test exercise the
// UploadStream error-classification branch through the *real* auth path, not a
// stub. CI sets the env var; local `go test ./...` skips and stays green.
func newUploadTestEnv(t *testing.T) (*sql.DB, string, string, int) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping DB-backed upload test")
	}
	db, err := data.InitDB(dsn)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	userID := uuid.NewString()
	if err := data.UpsertUser(ctx, db, userID, userID+"@test.local"); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	// Fresh owner per run → any slug is free (slugs are unique per owner).
	proj, err := data.CreateProject(ctx, db, userID, "", "GP413 "+userID[:8], nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	item, err := data.CreateItem(ctx, db, proj.ID, userID, data.ItemKindTask, "target", nil)
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	return db, userID, proj.Slug, item.Sequence
}

// postUpload drives HandleCollection exactly as the router+auth middleware
// would: path values set, the authenticated user injected into the context.
func postUpload(t *testing.T, h *AttachmentsHandler, userID, slug string, seq int) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "oversized.bin")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fw.Write([]byte("small body — the fake store decides the outcome")); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost,
		"/v1/projects/"+slug+"/items/"+strconv.Itoa(seq)+"/attachments", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = req.WithContext(context.WithValue(req.Context(), userIDKey, userID))
	req.SetPathValue("slug", slug)
	req.SetPathValue("seq", strconv.Itoa(seq))

	w := httptest.NewRecorder()
	h.HandleCollection(w, req)
	return w
}

// TestUpload_OversizedMapsTo413 pins the size-cap classification: when the
// store returns a %w-wrapped *http.MaxBytesError (what http.MaxBytesReader
// yields once the cap trips mid-stream), the handler must answer 413, not 500.
func TestUpload_OversizedMapsTo413(t *testing.T) {
	db, userID, slug, seq := newUploadTestEnv(t)
	store := storagetest.NewFake()
	store.UploadErr = fmt.Errorf("storage: upload %q: %w", "k",
		&http.MaxBytesError{Limit: MaxAttachmentBytes})

	w := postUpload(t, NewAttachmentsHandler(db, store), userID, slug, seq)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body=%s", w.Code, w.Body.String())
	}
}

// TestUpload_GenericStoreErrorMapsTo500 is the discrimination control: a
// non-MaxBytes store error must NOT be misclassified as 413.
func TestUpload_GenericStoreErrorMapsTo500(t *testing.T) {
	db, userID, slug, seq := newUploadTestEnv(t)
	store := storagetest.NewFake()
	store.UploadErr = fmt.Errorf("s3 exploded")

	w := postUpload(t, NewAttachmentsHandler(db, store), userID, slug, seq)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
	}
}
