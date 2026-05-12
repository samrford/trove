package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"trove/backend/internal/data"
)

type ItemsHandler struct {
	db *sql.DB
}

func NewItemsHandler(db *sql.DB) *ItemsHandler {
	return &ItemsHandler{db: db}
}

// HandleCollection serves /v1/projects/{slug}/items — GET (list) and POST (create).
func (h *ItemsHandler) HandleCollection(w http.ResponseWriter, r *http.Request) {
	slugOrID := r.PathValue("slug")
	if slugOrID == "" {
		http.Error(w, `{"error":"Project identifier required"}`, http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.list(w, r, slugOrID)
	case http.MethodPost:
		h.create(w, r, slugOrID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleByID serves /v1/projects/{slug}/items/{seq} — GET, PATCH, DELETE.
func (h *ItemsHandler) HandleByID(w http.ResponseWriter, r *http.Request) {
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

	switch r.Method {
	case http.MethodGet:
		h.get(w, r, slugOrID, seq)
	case http.MethodPatch:
		h.update(w, r, slugOrID, seq)
	case http.MethodDelete:
		h.delete(w, r, slugOrID, seq)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// resolveProject loads the project and ensures the user has access.
// Returns ErrNoRows → 404; OwnerID mismatch is not checked here (caller decides).
func (h *ItemsHandler) resolveProject(w http.ResponseWriter, r *http.Request, slugOrID string) (*data.Project, bool) {
	userID := GetUserID(r.Context())
	project, err := data.GetProjectForUser(r.Context(), h.db, userID, slugOrID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, `{"error":"Project not found"}`, http.StatusNotFound)
			return nil, false
		}
		log.Printf("GetProjectForUser: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return nil, false
	}
	return project, true
}

// requireOwner gates write actions to the project owner. v1 is personal-first;
// fine-grained role checks (contributor/viewer) land alongside groups in 00007.
func requireOwner(w http.ResponseWriter, project *data.Project, userID string) bool {
	if project.OwnerID != userID {
		http.Error(w, `{"error":"Only the project owner can do that"}`, http.StatusForbidden)
		return false
	}
	return true
}

func (h *ItemsHandler) list(w http.ResponseWriter, r *http.Request, slugOrID string) {
	project, ok := h.resolveProject(w, r, slugOrID)
	if !ok {
		return
	}

	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if kind != "" && !data.IsValidItemKind(kind) {
		http.Error(w, `{"error":"Invalid kind"}`, http.StatusBadRequest)
		return
	}
	if status != "" && !data.IsValidItemStatus(status) {
		http.Error(w, `{"error":"Invalid status"}`, http.StatusBadRequest)
		return
	}

	items, err := data.ListItemsForProject(r.Context(), h.db, project.ID, kind, status)
	if err != nil {
		log.Printf("ListItemsForProject: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(items); err != nil {
		log.Printf("encode items: %v", err)
	}
}

func (h *ItemsHandler) create(w http.ResponseWriter, r *http.Request, slugOrID string) {
	project, ok := h.resolveProject(w, r, slugOrID)
	if !ok {
		return
	}
	userID := GetUserID(r.Context())
	if !requireOwner(w, project, userID) {
		return
	}

	var body struct {
		Kind  string  `json:"kind"`
		Title string  `json:"title"`
		Body  *string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	body.Title = strings.TrimSpace(body.Title)
	body.Kind = strings.TrimSpace(body.Kind)
	if body.Title == "" {
		http.Error(w, `{"error":"Title is required"}`, http.StatusBadRequest)
		return
	}
	if !data.IsValidItemKind(body.Kind) {
		http.Error(w, `{"error":"Kind must be brainstorm or task"}`, http.StatusBadRequest)
		return
	}

	item, err := data.CreateItem(r.Context(), h.db, project.ID, userID,
		data.ItemKind(body.Kind), body.Title, body.Body)
	if err != nil {
		log.Printf("CreateItem: %v", err)
		http.Error(w, `{"error":"Failed to create item"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(item); err != nil {
		log.Printf("encode item: %v", err)
	}
}

func (h *ItemsHandler) get(w http.ResponseWriter, r *http.Request, slugOrID string, seq int) {
	project, ok := h.resolveProject(w, r, slugOrID)
	if !ok {
		return
	}

	item, err := data.GetItemBySequence(r.Context(), h.db, project.ID, seq)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, `{"error":"Item not found"}`, http.StatusNotFound)
			return
		}
		log.Printf("GetItemBySequence: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(item); err != nil {
		log.Printf("encode item: %v", err)
	}
}

func (h *ItemsHandler) update(w http.ResponseWriter, r *http.Request, slugOrID string, seq int) {
	project, ok := h.resolveProject(w, r, slugOrID)
	if !ok {
		return
	}
	userID := GetUserID(r.Context())
	if !requireOwner(w, project, userID) {
		return
	}

	existing, err := data.GetItemBySequence(r.Context(), h.db, project.ID, seq)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, `{"error":"Item not found"}`, http.StatusNotFound)
			return
		}
		log.Printf("GetItemBySequence: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}

	// All fields optional — only present keys are applied. Using json.RawMessage
	// + a presence map would be tidier, but pointer-fields keeps it boring.
	var body struct {
		Title    *string  `json:"title"`
		Body     *string  `json:"body"`
		Kind     *string  `json:"kind"`
		Status   *string  `json:"status"`
		Position *float64 `json:"position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	patch := data.ItemPatch{}
	if body.Title != nil {
		title := strings.TrimSpace(*body.Title)
		if title == "" {
			http.Error(w, `{"error":"Title cannot be empty"}`, http.StatusBadRequest)
			return
		}
		patch.Title = &title
	}
	if body.Body != nil {
		// Allow clearing — store empty string as NULL.
		trimmed := strings.TrimSpace(*body.Body)
		var bodyVal *string
		if trimmed != "" {
			bodyVal = &trimmed
		}
		patch.Body = &bodyVal
	}
	if body.Kind != nil {
		if !data.IsValidItemKind(*body.Kind) {
			http.Error(w, `{"error":"Invalid kind"}`, http.StatusBadRequest)
			return
		}
		k := data.ItemKind(*body.Kind)
		patch.Kind = &k
	}
	if body.Status != nil {
		if !data.IsValidItemStatus(*body.Status) {
			http.Error(w, `{"error":"Invalid status"}`, http.StatusBadRequest)
			return
		}
		s := data.ItemStatus(*body.Status)
		patch.Status = &s
	}
	if body.Position != nil {
		patch.Position = body.Position
	}

	updated, err := data.UpdateItem(r.Context(), h.db, existing.ID, patch)
	if err != nil {
		log.Printf("UpdateItem: %v", err)
		http.Error(w, `{"error":"Failed to update item"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(updated); err != nil {
		log.Printf("encode item: %v", err)
	}
}

func (h *ItemsHandler) delete(w http.ResponseWriter, r *http.Request, slugOrID string, seq int) {
	project, ok := h.resolveProject(w, r, slugOrID)
	if !ok {
		return
	}
	userID := GetUserID(r.Context())
	if !requireOwner(w, project, userID) {
		return
	}

	existing, err := data.GetItemBySequence(r.Context(), h.db, project.ID, seq)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, `{"error":"Item not found"}`, http.StatusNotFound)
			return
		}
		log.Printf("GetItemBySequence: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}

	if err := data.DeleteItem(r.Context(), h.db, existing.ID); err != nil {
		log.Printf("DeleteItem: %v", err)
		http.Error(w, `{"error":"Failed to delete item"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
