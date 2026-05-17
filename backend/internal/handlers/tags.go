package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"trove/backend/internal/data"
)

type TagsHandler struct {
	db *sql.DB
}

func NewTagsHandler(db *sql.DB) *TagsHandler {
	return &TagsHandler{db: db}
}

// HandleCollection serves /v1/tags — GET (list) and POST (create).
func (h *TagsHandler) HandleCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.list(w, r)
	case http.MethodPost:
		h.create(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleCheckSlug serves GET /v1/tags/check-slug?slug=foo. Returns
// {available: bool}. Registered separately so the literal path doesn't
// collide with the {slugOrID} matcher.
func (h *TagsHandler) HandleCheckSlug(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID := GetUserID(r.Context())
	slug := strings.TrimSpace(r.URL.Query().Get("slug"))

	w.Header().Set("Content-Type", "application/json")
	if !data.IsValidSlug(slug) {
		json.NewEncoder(w).Encode(map[string]any{"available": false, "reason": "invalid"})
		return
	}
	exists, err := data.TagSlugExistsForOwner(r.Context(), h.db, userID, slug)
	if err != nil {
		log.Printf("TagSlugExistsForOwner: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"available": !exists})
}

// HandleByID serves /v1/tags/{slugOrID} — GET, PATCH, DELETE.
func (h *TagsHandler) HandleByID(w http.ResponseWriter, r *http.Request) {
	slugOrID := strings.TrimPrefix(r.URL.Path, "/v1/tags/")
	if slugOrID == "" || strings.Contains(slugOrID, "/") {
		http.Error(w, "Tag identifier required", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.get(w, r, slugOrID)
	case http.MethodPatch:
		h.update(w, r, slugOrID)
	case http.MethodDelete:
		h.delete(w, r, slugOrID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleItemsForTag serves GET /v1/tags/{slug}/items — items using this tag.
func (h *TagsHandler) HandleItemsForTag(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	slugOrID := r.PathValue("slug")
	if slugOrID == "" {
		http.Error(w, `{"error":"Tag identifier required"}`, http.StatusBadRequest)
		return
	}

	userID := GetUserID(r.Context())
	tag, err := data.GetTagForUser(r.Context(), h.db, userID, slugOrID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, `{"error":"Tag not found"}`, http.StatusNotFound)
			return
		}
		log.Printf("GetTagForUser: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}

	items, err := data.ListItemsForTag(r.Context(), h.db, tag.ID)
	if err != nil {
		log.Printf("ListItemsForTag: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(items); err != nil {
		log.Printf("encode items: %v", err)
	}
}

// HandleTagsForProject serves GET /v1/projects/{slug}/tags — tags used in
// this project (for the filter sidebar). Returns TagWithCount[].
func (h *TagsHandler) HandleTagsForProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	slugOrID := r.PathValue("slug")
	if slugOrID == "" {
		http.Error(w, `{"error":"Project identifier required"}`, http.StatusBadRequest)
		return
	}

	userID := GetUserID(r.Context())
	project, err := data.GetProjectForUser(r.Context(), h.db, userID, slugOrID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, `{"error":"Project not found"}`, http.StatusNotFound)
			return
		}
		log.Printf("GetProjectForUser: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}

	tags, err := data.ListTagsUsedInProject(r.Context(), h.db, project.ID)
	if err != nil {
		log.Printf("ListTagsUsedInProject: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(tags); err != nil {
		log.Printf("encode tags: %v", err)
	}
}

// requireOwnedTag fetches a tag and confirms the caller owns it. Writes the
// 404/403/500 response and returns ok=false on any failure.
func (h *TagsHandler) requireOwnedTag(w http.ResponseWriter, r *http.Request, userID, slugOrID, verb string) (*data.Tag, bool) {
	tag, err := data.GetTagForUser(r.Context(), h.db, userID, slugOrID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, `{"error":"Tag not found"}`, http.StatusNotFound)
			return nil, false
		}
		log.Printf("GetTagForUser: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return nil, false
	}
	if tag.UserID == nil || *tag.UserID != userID {
		http.Error(w, `{"error":"Only the tag owner can `+verb+` this tag"}`, http.StatusForbidden)
		return nil, false
	}
	return tag, true
}

func (h *TagsHandler) list(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r.Context())
	tags, err := data.ListTagsForUser(r.Context(), h.db, userID)
	if err != nil {
		log.Printf("ListTagsForUser: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(tags); err != nil {
		log.Printf("encode tags: %v", err)
	}
}

func (h *TagsHandler) create(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r.Context())

	var body struct {
		Slug        string  `json:"slug"`
		Name        string  `json:"name"`
		Description *string `json:"description"`
		Colour      *string `json:"colour"`
		Icon        *string `json:"icon"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	body.Name = strings.TrimSpace(body.Name)
	body.Slug = strings.TrimSpace(body.Slug)
	if body.Name == "" {
		http.Error(w, `{"error":"Name is required"}`, http.StatusBadRequest)
		return
	}
	if body.Slug != "" && !data.IsValidSlug(body.Slug) {
		http.Error(w, `{"error":"Slug must contain only lowercase letters, numbers, and dashes"}`, http.StatusBadRequest)
		return
	}

	var tag *data.Tag
	var created bool
	err := data.WithRetry(r.Context(), h.db, func(tx *sql.Tx) error {
		var e error
		tag, created, e = data.FindOrCreateTag(r.Context(), tx, userID,
			body.Name, body.Slug, body.Description, body.Icon, body.Colour)
		return e
	})
	if err != nil {
		if errors.Is(err, data.ErrTagSlugTaken) {
			http.Error(w, `{"error":"That slug is already taken — try another."}`, http.StatusConflict)
			return
		}
		log.Printf("FindOrCreateTag: %v", err)
		http.Error(w, `{"error":"Failed to create tag"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if created {
		w.WriteHeader(http.StatusCreated)
	} else {
		// Auto-merge hit: return 200 OK with the existing tag so the client
		// can distinguish "new" vs "you already had this".
		w.WriteHeader(http.StatusOK)
	}
	if err := json.NewEncoder(w).Encode(tag); err != nil {
		log.Printf("encode tag: %v", err)
	}
}

func (h *TagsHandler) get(w http.ResponseWriter, r *http.Request, slugOrID string) {
	userID := GetUserID(r.Context())
	tag, err := data.GetTagForUser(r.Context(), h.db, userID, slugOrID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, `{"error":"Tag not found"}`, http.StatusNotFound)
			return
		}
		log.Printf("GetTagForUser: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(tag); err != nil {
		log.Printf("encode tag: %v", err)
	}
}

func (h *TagsHandler) update(w http.ResponseWriter, r *http.Request, slugOrID string) {
	userID := GetUserID(r.Context())

	existing, ok := h.requireOwnedTag(w, r, userID, slugOrID, "edit")
	if !ok {
		return
	}

	var body struct {
		Name        string  `json:"name"`
		Slug        string  `json:"slug"`
		Description *string `json:"description"`
		Colour      *string `json:"colour"`
		Icon        *string `json:"icon"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	body.Name = strings.TrimSpace(body.Name)
	body.Slug = strings.TrimSpace(body.Slug)
	if body.Name == "" {
		http.Error(w, `{"error":"Name is required"}`, http.StatusBadRequest)
		return
	}
	if body.Slug != "" && !data.IsValidSlug(body.Slug) {
		http.Error(w, `{"error":"Slug must contain only lowercase letters, numbers, and dashes"}`, http.StatusBadRequest)
		return
	}

	updated, err := data.UpdateTag(r.Context(), h.db, existing.ID, body.Name, body.Slug,
		body.Description, body.Icon, body.Colour)
	if err != nil {
		if errors.Is(err, data.ErrTagSlugTaken) {
			http.Error(w, `{"error":"That slug is already taken — try another."}`, http.StatusConflict)
			return
		}
		log.Printf("UpdateTag: %v", err)
		http.Error(w, `{"error":"Failed to update tag"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(updated); err != nil {
		log.Printf("encode tag: %v", err)
	}
}

func (h *TagsHandler) delete(w http.ResponseWriter, r *http.Request, slugOrID string) {
	userID := GetUserID(r.Context())

	existing, ok := h.requireOwnedTag(w, r, userID, slugOrID, "delete")
	if !ok {
		return
	}

	if err := data.DeleteTag(r.Context(), h.db, existing.ID); err != nil {
		log.Printf("DeleteTag: %v", err)
		http.Error(w, `{"error":"Failed to delete tag"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
