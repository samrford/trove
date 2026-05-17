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

// GPhotosHandler hosts the one Google Photos route that needs Trove context:
// StartImport. Session create/poll need no per-item context, so the picker
// library's handlers are mounted directly for those — only the import has to
// authorise the destination and attach it as server-derived metadata.
type GPhotosHandler struct {
	db *sql.DB
	c  *photopicker.Client
}

func NewGPhotosHandler(db *sql.DB, client *photopicker.Client) *GPhotosHandler {
	return &GPhotosHandler{db: db, c: client}
}

// HandleStartImport serves POST
// /v1/projects/{slug}/items/{seq}/google-photos/sessions/{sid}/import.
//
// The destination is derived entirely server-side: we resolve and
// ownership-check the project/item from the authenticated request, then start
// the import with those IDs as metadata. The browser never supplies the
// destination, so it can't be spoofed onto someone else's item. The picker
// library threads the metadata back to NewTroveSink via
// DownloadedPhoto.JobMetadata.
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

	jobID, err := h.c.StartImport(r.Context(), userID, sessionID, map[string]string{
		"project_id": project.ID,
		"item_id":    item.ID,
	})
	if err != nil {
		log.Printf("StartImport: %v", err)
		http.Error(w, `{"error":"Failed to start import"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"importJobId": jobID}); err != nil {
		log.Printf("encode importJobId: %v", err)
	}
}

// resolveItem loads and ownership-checks the project/item named in the path.
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

// NewTroveSink streams each downloaded photo to storage and records an
// attachment row, routed to the destination item via the server-derived
// JobMetadata set at StartImport. Missing metadata is a hard error — it would
// only happen if an import were started outside HandleStartImport.
func NewTroveSink(db *sql.DB, store storage.FileStore) photopicker.PhotoSink {
	return photopicker.SinkFunc(func(ctx context.Context, userID, jobID string, p photopicker.DownloadedPhoto) (string, error) {
		projectID := p.JobMetadata["project_id"]
		itemID := p.JobMetadata["item_id"]
		if projectID == "" || itemID == "" {
			return "", fmt.Errorf("job %s: missing project_id/item_id in metadata", jobID)
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
			projectID, itemID, uuid.NewString(), filepath.Ext(filename))

		size, err := store.UploadStream(ctx, key, p.Bytes, contentType)
		if err != nil {
			return "", fmt.Errorf("upload: %w", err)
		}

		var att *data.Attachment
		err = data.WithRetry(ctx, db, func(tx *sql.Tx) error {
			var e error
			att, e = data.CreateAttachment(ctx, tx, data.CreateAttachmentParams{
				ProjectID:   projectID,
				ItemID:      &itemID,
				StorageKey:  key,
				Filename:    filename,
				ContentType: contentType,
				SizeBytes:   size,
				Source:      data.AttachmentSourceGooglePhotos,
				UploaderID:  userID,
			})
			if e != nil {
				return e
			}
			it, e := data.GetItemByID(ctx, db, itemID)
			if e != nil {
				return e
			}
			_, e = data.LogActivity(ctx, tx, data.ActivityInput{
				ProjectID: projectID,
				ItemID:    &itemID,
				ActorID:   userID,
				Action:    data.ActivityAttachmentAdded,
				Payload: map[string]any{
					"item":       itemSnapshot(it),
					"attachment": map[string]any{"filename": filename, "size_bytes": size, "source": data.AttachmentSourceGooglePhotos},
				},
			})
			return e
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
// the corresponding extension so the stored filename is at least vaguely
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
