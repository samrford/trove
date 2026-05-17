package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/google/uuid"

	"trove/backend/internal/data"
	"trove/backend/internal/data/storage"
)

// MaxAttachmentBytes caps a single upload at 25MB
const MaxAttachmentBytes int64 = 25 * 1024 * 1024

// SignedURLTTL is how long the attachment URLs returned in item GET responses
// stay valid.
const SignedURLTTL = 1 * time.Hour

type AttachmentsHandler struct {
	db    *sql.DB
	store storage.FileStore
}

func NewAttachmentsHandler(db *sql.DB, store storage.FileStore) *AttachmentsHandler {
	return &AttachmentsHandler{db: db, store: store}
}

// AttachmentResponse is the JSON shape returned to clients. Wraps the data
// row with a freshly-signed URL so the browser can fetch the blob directly
// from storage without proxying through the backend.
type AttachmentResponse struct {
	ID          string                `json:"id"`
	ProjectID   string                `json:"project_id"`
	ItemID      *string               `json:"item_id"`
	Filename    string                `json:"filename"`
	ContentType string                `json:"content_type"`
	SizeBytes   int64                 `json:"size_bytes"`
	Source      data.AttachmentSource `json:"source"`
	UploaderID  string                `json:"uploader_id"`
	CreatedAt   time.Time             `json:"created_at"`
	URL         string                `json:"url"`
}

// SignAttachments converts data.Attachment rows into the response shape with
// fresh signed URLs. Exported because items.go calls it when embedding
// attachments in item responses.
func SignAttachments(ctx context.Context, store storage.FileStore, attachments []data.Attachment) ([]AttachmentResponse, error) {
	out := make([]AttachmentResponse, len(attachments))
	for i, a := range attachments {
		url, err := store.SignedURL(ctx, a.StorageKey, SignedURLTTL)
		if err != nil {
			return nil, fmt.Errorf("sign %s: %w", a.ID, err)
		}
		out[i] = attachmentToResponse(a, url)
	}
	return out, nil
}

func attachmentToResponse(a data.Attachment, url string) AttachmentResponse {
	return AttachmentResponse{
		ID:          a.ID,
		ProjectID:   a.ProjectID,
		ItemID:      a.ItemID,
		Filename:    a.Filename,
		ContentType: a.ContentType,
		SizeBytes:   a.SizeBytes,
		Source:      a.Source,
		UploaderID:  a.UploaderID,
		CreatedAt:   a.CreatedAt,
		URL:         url,
	}
}

// HandleCollection serves /v1/projects/{slug}/items/{seq}/attachments — POST.
func (h *AttachmentsHandler) HandleCollection(w http.ResponseWriter, r *http.Request) {
	slugOrID := r.PathValue("slug")
	seqStr := r.PathValue("seq")
	if slugOrID == "" || seqStr == "" {
		http.Error(w, `{"error":"Project + item required"}`, http.StatusBadRequest)
		return
	}
	seq, err := strconv.Atoi(seqStr)
	if err != nil || seq < 1 {
		http.Error(w, `{"error":"Invalid item sequence"}`, http.StatusBadRequest)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.upload(w, r, slugOrID, seq)
}

// HandleByID serves /v1/projects/{slug}/items/{seq}/attachments/{id} — DELETE.
func (h *AttachmentsHandler) HandleByID(w http.ResponseWriter, r *http.Request) {
	slugOrID := r.PathValue("slug")
	seqStr := r.PathValue("seq")
	attID := r.PathValue("id")
	if slugOrID == "" || seqStr == "" || attID == "" {
		http.Error(w, `{"error":"Project + item + attachment required"}`, http.StatusBadRequest)
		return
	}
	seq, err := strconv.Atoi(seqStr)
	if err != nil || seq < 1 {
		http.Error(w, `{"error":"Invalid item sequence"}`, http.StatusBadRequest)
		return
	}

	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.delete(w, r, slugOrID, seq, attID)
}

func (h *AttachmentsHandler) upload(w http.ResponseWriter, r *http.Request, slugOrID string, seq int) {
	userID := GetUserID(r.Context())
	project, item, ok := h.resolveItem(w, r, slugOrID, seq)
	if !ok {
		return
	}
	if !requireOwner(w, project, userID) {
		return
	}

	// Enforce size cap at the request-body layer — both the multipart reader
	// and any consumers downstream see a wrapped reader that returns an error
	// after the limit. The browser sees a 413 rather than a half-uploaded blob.
	r.Body = http.MaxBytesReader(w, r.Body, MaxAttachmentBytes)

	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, `{"error":"Expected multipart/form-data"}`, http.StatusBadRequest)
		return
	}

	// Walk parts until we find the file. Ignore non-file form fields so a
	// future flag in the same form (e.g. caption=...) wouldn't break us.
	var part *multipart.Part
	for {
		p, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			httpMultipartError(w, err)
			return
		}
		if p.FormName() == "file" {
			part = p
			break
		}
		p.Close()
	}
	if part == nil {
		http.Error(w, `{"error":"Missing 'file' field"}`, http.StatusBadRequest)
		return
	}
	defer part.Close()

	filename := part.FileName()
	if filename == "" {
		http.Error(w, `{"error":"Missing filename"}`, http.StatusBadRequest)
		return
	}

	// Sniff content type from the first 512 bytes, falling back to the form
	// header. We read the bytes into a buffer and stitch with the rest of the
	// stream so the S3 SDK sees the whole file.
	peek := make([]byte, 512)
	n, err := io.ReadFull(part, peek)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		httpMultipartError(w, err)
		return
	}
	peek = peek[:n]
	contentType := http.DetectContentType(peek)
	// DetectContentType returns "application/octet-stream" when it can't tell;
	// in that case, fall back to the part's declared header (still better than
	// the catch-all).
	if contentType == "application/octet-stream" {
		if declared := part.Header.Get("Content-Type"); declared != "" {
			contentType = declared
		}
	}

	storageKey := buildStorageKey(project.ID, item.ID, filename)
	body := io.MultiReader(bytes.NewReader(peek), part)

	size, err := h.store.UploadStream(r.Context(), storageKey, body, contentType)
	if err != nil {
		// MaxBytesReader returns *http.MaxBytesError once exceeded; surface a
		// 413 instead of a generic 500.
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, fmt.Sprintf(`{"error":"File exceeds %d byte cap"}`, MaxAttachmentBytes), http.StatusRequestEntityTooLarge)
			return
		}
		log.Printf("UploadStream: %v", err)
		http.Error(w, `{"error":"Upload failed"}`, http.StatusInternalServerError)
		return
	}

	itemID := item.ID
	var att *data.Attachment
	err = data.WithRetry(r.Context(), h.db, func(tx *sql.Tx) error {
		var e error
		att, e = data.CreateAttachment(r.Context(), tx, data.CreateAttachmentParams{
			ProjectID:   project.ID,
			ItemID:      &itemID,
			StorageKey:  storageKey,
			Filename:    filename,
			ContentType: contentType,
			SizeBytes:   size,
			Source:      data.AttachmentSourceUpload,
			UploaderID:  userID,
		})
		if e != nil {
			return e
		}
		_, e = data.LogActivity(r.Context(), tx, data.ActivityInput{
			ProjectID: project.ID,
			ItemID:    &itemID,
			ActorID:   userID,
			Action:    data.ActivityAttachmentAdded,
			Payload: map[string]any{
				"item":       itemSnapshot(item),
				"attachment": map[string]any{"filename": filename, "size_bytes": size, "source": data.AttachmentSourceUpload},
			},
		})
		return e
	})
	if err != nil {
		// DB insert failed after upload succeeded — purge the orphan now so we
		// don't rely on the daily sweep to clean it up.
		if delErr := h.store.Delete(r.Context(), storageKey); delErr != nil {
			log.Printf("rollback delete %s: %v", storageKey, delErr)
		}
		log.Printf("CreateAttachment: %v", err)
		http.Error(w, `{"error":"Failed to record attachment"}`, http.StatusInternalServerError)
		return
	}

	url, err := h.store.SignedURL(r.Context(), att.StorageKey, SignedURLTTL)
	if err != nil {
		log.Printf("SignedURL: %v", err)
		http.Error(w, `{"error":"Attachment created but URL signing failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(attachmentToResponse(*att, url)); err != nil {
		log.Printf("encode attachment: %v", err)
	}
}

func (h *AttachmentsHandler) delete(w http.ResponseWriter, r *http.Request, slugOrID string, seq int, attID string) {
	userID := GetUserID(r.Context())
	project, item, ok := h.resolveItem(w, r, slugOrID, seq)
	if !ok {
		return
	}
	if !requireOwner(w, project, userID) {
		return
	}

	// Confirm the attachment belongs to this item — guards against an attacker
	// constructing a URL that references someone else's attachment.
	att, err := data.GetAttachmentByID(r.Context(), h.db, attID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, `{"error":"Attachment not found"}`, http.StatusNotFound)
			return
		}
		log.Printf("GetAttachmentByID: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}
	if att.ItemID == nil || *att.ItemID != item.ID {
		http.Error(w, `{"error":"Attachment not found"}`, http.StatusNotFound)
		return
	}

	var storageKey string
	err = data.WithRetry(r.Context(), h.db, func(tx *sql.Tx) error {
		var e error
		storageKey, e = data.DeleteAttachment(r.Context(), tx, attID)
		if e != nil {
			return e
		}
		_, e = data.LogActivity(r.Context(), tx, data.ActivityInput{
			ProjectID: project.ID,
			ItemID:    &item.ID,
			ActorID:   userID,
			Action:    data.ActivityAttachmentRemoved,
			Payload: map[string]any{
				"item":       itemSnapshot(item),
				"attachment": map[string]any{"filename": att.Filename},
			},
		})
		return e
	})
	if err != nil {
		if errors.Is(err, data.ErrAttachmentNotFound) {
			http.Error(w, `{"error":"Attachment not found"}`, http.StatusNotFound)
			return
		}
		log.Printf("DeleteAttachment: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}

	if err := h.store.Delete(r.Context(), storageKey); err != nil {
		// Row is already gone; log it so the orphan sweep notices later.
		log.Printf("storage.Delete %s (orphaned, will sweep): %v", storageKey, err)
	}

	w.WriteHeader(http.StatusNoContent)
}

// resolveItem loads the project + item in one go for handlers that need both.
func (h *AttachmentsHandler) resolveItem(w http.ResponseWriter, r *http.Request, slugOrID string, seq int) (*data.Project, *data.Item, bool) {
	userID := GetUserID(r.Context())
	project, err := data.GetProjectForUser(r.Context(), h.db, userID, slugOrID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, `{"error":"Project not found"}`, http.StatusNotFound)
			return nil, nil, false
		}
		log.Printf("GetProjectForUser: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return nil, nil, false
	}
	item, err := data.GetItemBySequence(r.Context(), h.db, project.ID, seq)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, `{"error":"Item not found"}`, http.StatusNotFound)
			return nil, nil, false
		}
		log.Printf("GetItemBySequence: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return nil, nil, false
	}
	return project, item, true
}

// buildStorageKey returns the canonical object key for a new attachment.
// Includes the original extension so the signed URL hints at the file type to
// browsers / consumers.
func buildStorageKey(projectID, itemID, filename string) string {
	ext := filepath.Ext(filename)
	return fmt.Sprintf("projects/%s/items/%s/%s%s", projectID, itemID, uuid.NewString(), ext)
}

// httpMultipartError maps the common multipart parse failures (including the
// MaxBytesReader trip) to sensible HTTP codes.
func httpMultipartError(w http.ResponseWriter, err error) {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		http.Error(w, fmt.Sprintf(`{"error":"File exceeds %d byte cap"}`, MaxAttachmentBytes), http.StatusRequestEntityTooLarge)
		return
	}
	log.Printf("multipart read: %v", err)
	http.Error(w, `{"error":"Malformed multipart request"}`, http.StatusBadRequest)
}

