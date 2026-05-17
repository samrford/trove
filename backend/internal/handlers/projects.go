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

type ProjectsHandler struct {
	db *sql.DB
}

func NewProjectsHandler(db *sql.DB) *ProjectsHandler {
	return &ProjectsHandler{db: db}
}

// HandleCollection serves /v1/projects — GET (list) and POST (create).
func (h *ProjectsHandler) HandleCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.list(w, r)
	case http.MethodPost:
		h.create(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleCheckSlug serves GET /v1/projects/check-slug?slug=foo. Returns
// {available: bool} — per-owner, since slug uniqueness is per owner_id.
// Registered as a discrete route so the literal "check-slug" path doesn't
// clash with HandleByID's {slugOrID} matcher.
func (h *ProjectsHandler) HandleCheckSlug(w http.ResponseWriter, r *http.Request) {
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

	exists, err := data.SlugExistsForOwner(r.Context(), h.db, userID, slug)
	if err != nil {
		log.Printf("SlugExistsForOwner: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]any{"available": !exists})
}

// HandleByID serves /v1/projects/{slugOrID} — GET (fetch one).
func (h *ProjectsHandler) HandleByID(w http.ResponseWriter, r *http.Request) {
	slugOrID := strings.TrimPrefix(r.URL.Path, "/v1/projects/")
	if slugOrID == "" || strings.Contains(slugOrID, "/") {
		http.Error(w, "Project identifier required", http.StatusBadRequest)
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

func (h *ProjectsHandler) list(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r.Context())

	projects, err := data.ListProjectsForUser(r.Context(), h.db, userID)
	if err != nil {
		log.Printf("ListProjectsForUser: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(projects); err != nil {
		log.Printf("encode projects: %v", err)
	}
}

func (h *ProjectsHandler) create(w http.ResponseWriter, r *http.Request) {
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

	var project *data.Project
	err := data.WithRetry(r.Context(), h.db, func(tx *sql.Tx) error {
		var err error
		project, err = data.CreateProject(r.Context(), tx, userID, body.Slug, body.Name,
			body.Description, body.Colour, body.Icon)
		if err != nil {
			return err
		}
		_, err = data.LogActivity(r.Context(), tx, data.ActivityInput{
			ProjectID: project.ID,
			ActorID:   userID,
			Action:    data.ActivityProjectCreated,
			Payload:   map[string]any{"project": projectSnapshot(project)},
		})
		return err
	})
	if err != nil {
		if errors.Is(err, data.ErrSlugTaken) {
			http.Error(w, `{"error":"That slug is already taken — try another."}`, http.StatusConflict)
			return
		}
		log.Printf("CreateProject: %v", err)
		http.Error(w, `{"error":"Failed to create project"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(project); err != nil {
		log.Printf("encode project: %v", err)
	}
}

func (h *ProjectsHandler) get(w http.ResponseWriter, r *http.Request, slugOrID string) {
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

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(project); err != nil {
		log.Printf("encode project: %v", err)
	}
}

// update handles PATCH /v1/projects/{slugOrID}. Owner-only. The body is treated
// as a full replacement of the editable fields (UI sends all four every time);
// `null` for description/colour/icon explicitly clears that field.
func (h *ProjectsHandler) update(w http.ResponseWriter, r *http.Request, slugOrID string) {
	userID := GetUserID(r.Context())

	existing, err := data.GetProjectForUser(r.Context(), h.db, userID, slugOrID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, `{"error":"Project not found"}`, http.StatusNotFound)
			return
		}
		log.Printf("GetProjectForUser: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}
	if existing.OwnerID != userID {
		http.Error(w, `{"error":"Only the project owner can edit this project"}`, http.StatusForbidden)
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

	var updated *data.Project
	err = data.WithRetry(r.Context(), h.db, func(tx *sql.Tx) error {
		var e error
		updated, e = data.UpdateProject(r.Context(), tx, existing.ID, body.Name, body.Slug,
			body.Description, body.Colour, body.Icon)
		if e != nil {
			return e
		}
		diff := projectDiff(existing, updated)
		if len(diff) == 0 {
			return nil
		}
		_, e = data.LogActivity(r.Context(), tx, data.ActivityInput{
			ProjectID: existing.ID,
			ActorID:   userID,
			Action:    data.ActivityProjectUpdated,
			Payload:   map[string]any{"project": projectSnapshot(updated), "diff": diff},
		})
		return e
	})
	if err != nil {
		if errors.Is(err, data.ErrSlugTaken) {
			http.Error(w, `{"error":"That slug is already taken — try another."}`, http.StatusConflict)
			return
		}
		log.Printf("UpdateProject: %v", err)
		http.Error(w, `{"error":"Failed to update project"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(updated); err != nil {
		log.Printf("encode project: %v", err)
	}
}

// delete handles DELETE /v1/projects/{slugOrID}. Owner-only. Hard delete —
// dependent rows (members, sequences, future items/tags) cascade.
func (h *ProjectsHandler) delete(w http.ResponseWriter, r *http.Request, slugOrID string) {
	userID := GetUserID(r.Context())

	existing, err := data.GetProjectForUser(r.Context(), h.db, userID, slugOrID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, `{"error":"Project not found"}`, http.StatusNotFound)
			return
		}
		log.Printf("GetProjectForUser: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}
	if existing.OwnerID != userID {
		http.Error(w, `{"error":"Only the project owner can delete this project"}`, http.StatusForbidden)
		return
	}

	// No activity log, the project's own feed dies with it.
	if err := data.WithRetry(r.Context(), h.db, func(tx *sql.Tx) error {
		return data.DeleteProject(r.Context(), tx, existing.ID)
	}); err != nil {
		log.Printf("DeleteProject: %v", err)
		http.Error(w, `{"error":"Failed to delete project"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
