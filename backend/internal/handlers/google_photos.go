package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/google/uuid"
	photopicker "github.com/samrford/google-photos-picker"

	"trove/backend/internal/data"
	"trove/backend/internal/data/storage"
)

// GPhotosHandler wraps the photo-picker library's session/import flow so we
// can capture which Trove item each session targets. The actual OAuth and
// session lifecycle handlers come from `picker.NewHandlers(...)` and are
// mounted directly on the mux by main.
type GPhotosHandler struct {
	db    *sql.DB
	store storage.FileStore
	pp    *photopicker.Handlers
	c     *photopicker.Client
}

func NewGPhotosHandler(db *sql.DB, store storage.FileStore, pp *photopicker.Handlers, client *photopicker.Client) *GPhotosHandler {
	return &GPhotosHandler{db: db, store: store, pp: pp, c: client}
}

// HandleCreateSession serves POST /v1/projects/{slug}/items/{seq}/google-photos/sessions.
// Identical body to the picker lib's CreateSession but resolves the item
// first so the frontend gets a 404 before opening the Google picker UI for a
// missing item.
func (h *GPhotosHandler) HandleCreateSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, _, ok := h.resolveItem(w, r); !ok {
		return
	}
	h.pp.CreateSession()(w, r)
}

// HandleStartImport serves POST /v1/projects/{slug}/items/{seq}/google-photos/sessions/{sid}/import.
// Lets the picker library kick off the import worker, then records the
// resulting job_id alongside the destination item so the sink can route each
// photo to the right place.
func (h *GPhotosHandler) HandleStartImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	project, item, ok := h.resolveItem(w, r)
	if !ok {
		return
	}
	userID := GetUserID(r.Context())
	sessionID := r.PathValue("sid")
	if sessionID == "" {
		http.Error(w, `{"error":"Session ID required"}`, http.StatusBadRequest)
		return
	}

	// Call the client directly so we get the job_id back rather than going
	// through the library's handler which only writes it to the response.
	jobID, err := h.c.StartImport(r.Context(), userID, sessionID)
	if err != nil {
		log.Printf("StartImport: %v", err)
		http.Error(w, `{"error":"Failed to start import"}`, http.StatusInternalServerError)
		return
	}

	if err := data.CreateGPhotosImportTarget(r.Context(), h.db, data.GPhotosImportTarget{
		JobID:     jobID,
		ProjectID: project.ID,
		ItemID:    item.ID,
		UserID:    userID,
	}); err != nil {
		log.Printf("CreateGPhotosImportTarget: %v", err)
		http.Error(w, `{"error":"Failed to record import destination"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"importJobId": jobID}); err != nil {
		log.Printf("encode importJobId: %v", err)
	}
}

func (h *GPhotosHandler) resolveItem(w http.ResponseWriter, r *http.Request) (*data.Project, *data.Item, bool) {
	slugOrID := r.PathValue("slug")
	seqStr := r.PathValue("seq")
	if slugOrID == "" || seqStr == "" {
		http.Error(w, `{"error":"Project + item required"}`, http.StatusBadRequest)
		return nil, nil, false
	}
	seq, err := strconv.Atoi(seqStr)
	if err != nil || seq < 1 {
		http.Error(w, `{"error":"Invalid item sequence"}`, http.StatusBadRequest)
		return nil, nil, false
	}
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
	if !requireOwner(w, project, userID) {
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

// NewTroveSink returns a PhotoSink that streams downloaded photos to storage
// and inserts an attachments row for each. Routes by job_id via the
// gphotos_import_targets table.
func NewTroveSink(db *sql.DB, store storage.FileStore) photopicker.PhotoSink {
	return photopicker.SinkFunc(func(ctx context.Context, userID, jobID string, p photopicker.DownloadedPhoto) (string, error) {
		target, err := data.GetGPhotosImportTarget(ctx, db, jobID)
		if err != nil {
			return "", fmt.Errorf("lookup target: %w", err)
		}.
		if target.UserID != userID {
			return "", fmt.Errorf("user mismatch on job %s", jobID)
		}

		filename := p.Filename
		if filename == "" {
			filename = "google-photo" + extensionFromMime(p.MimeType)
		}
		contentType := p.MimeType
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		key := fmt.Sprintf("projects/%s/items/%s/%s%s",
			target.ProjectID, target.ItemID, uuid.NewString(), filepath.Ext(filename))

		size, err := store.UploadStream(ctx, key, p.Bytes, contentType)
		if err != nil {
			return "", fmt.Errorf("upload: %w", err)
		}

		itemID := target.ItemID
		att, err := data.CreateAttachment(ctx, db, data.CreateAttachmentParams{
			ProjectID:   target.ProjectID,
			ItemID:      &itemID,
			StorageKey:  key,
			Filename:    filename,
			ContentType: contentType,
			SizeBytes:   size,
			Source:      data.AttachmentSourceGooglePhotos,
			UploaderID:  userID,
		})
		if err != nil {
			if delErr := store.Delete(ctx, key); delErr != nil {
				log.Printf("rollback delete %s: %v", key, delErr)
			}
			return "", fmt.Errorf("record attachment: %w", err)
		}
		return att.ID, nil
	})
}

// extensionFromMime maps the handful of MIME types Google Photos serves into
// the corresponding extension so the resulting filename is at least vaguely
// useful. Unknown types fall back to .bin.
func extensionFromMime(mt string) string {
	switch mt {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/heic":
		return ".heic"
	case "image/webp":
		return ".webp"
	case "video/mp4":
		return ".mp4"
	case "video/quicktime":
		return ".mov"
	default:
		return ".bin"
	}
}
