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
	"trove/backend/internal/data/storage"
)

type ItemsHandler struct {
	db    *sql.DB
	store storage.FileStore
}

func NewItemsHandler(db *sql.DB, store storage.FileStore) *ItemsHandler {
	return &ItemsHandler{db: db, store: store}
}

// ItemResponse is the JSON shape returned for item GETs — wraps data.Item to
// add fresh signed URLs for each attachment. Embedding so all of Item's JSON
// fields are surfaced unchanged.
type ItemResponse struct {
	data.Item
	Attachments []AttachmentResponse `json:"attachments"`
}

// itemsToResponses batches the attachment-signing pass so list handlers don't
// repeat the wiring. Pure projection — never errors except on signing failure.
func (h *ItemsHandler) itemsToResponses(r *http.Request, items []data.Item) ([]ItemResponse, error) {
	ids := make([]string, len(items))
	for i := range items {
		ids[i] = items[i].ID
	}
	var byItem map[string][]data.Attachment
	if h.store != nil {
		var err error
		byItem, err = data.ListAttachmentsForItems(r.Context(), h.db, ids)
		if err != nil {
			return nil, err
		}
	}
	out := make([]ItemResponse, len(items))
	for i, item := range items {
		out[i].Item = item
		out[i].Attachments = []AttachmentResponse{}
		if h.store == nil {
			continue
		}
		if list := byItem[item.ID]; len(list) > 0 {
			signed, err := SignAttachments(r.Context(), h.store, list)
			if err != nil {
				return nil, err
			}
			out[i].Attachments = signed
		}
	}
	return out, nil
}

// itemToResponse is the single-item analogue. Cheaper than going through
// itemsToResponses for the common GET-by-id path.
func (h *ItemsHandler) itemToResponse(r *http.Request, item data.Item) (ItemResponse, error) {
	resp := ItemResponse{Item: item, Attachments: []AttachmentResponse{}}
	if h.store == nil {
		return resp, nil
	}
	list, err := data.ListAttachmentsForItem(r.Context(), h.db, item.ID)
	if err != nil {
		return resp, err
	}
	if len(list) > 0 {
		signed, err := SignAttachments(r.Context(), h.store, list)
		if err != nil {
			return resp, err
		}
		resp.Attachments = signed
	}
	return resp, nil
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

// HandleItemTags serves /v1/projects/{slug}/items/{seq}/tags — POST (attach).
func (h *ItemsHandler) HandleItemTags(w http.ResponseWriter, r *http.Request) {
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
	h.attachTag(w, r, slugOrID, seq)
}

// HandleItemTagByID serves /v1/projects/{slug}/items/{seq}/tags/{tagSlug} — DELETE.
func (h *ItemsHandler) HandleItemTagByID(w http.ResponseWriter, r *http.Request) {
	slugOrID := r.PathValue("slug")
	seqStr := r.PathValue("seq")
	tagSlug := r.PathValue("tagSlug")
	if slugOrID == "" || seqStr == "" || tagSlug == "" {
		http.Error(w, `{"error":"Project + item + tag required"}`, http.StatusBadRequest)
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
	h.detachTag(w, r, slugOrID, seq, tagSlug)
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

	q := r.URL.Query()
	kind := strings.TrimSpace(q.Get("kind"))
	status := strings.TrimSpace(q.Get("status"))
	tagSlugs := q["tag"]
	tagMode := strings.ToLower(strings.TrimSpace(q.Get("tag_mode")))

	if kind != "" && !data.IsValidItemKind(kind) {
		http.Error(w, `{"error":"Invalid kind"}`, http.StatusBadRequest)
		return
	}
	if status != "" && !data.IsValidItemStatus(status) {
		http.Error(w, `{"error":"Invalid status"}`, http.StatusBadRequest)
		return
	}
	if tagMode != "" && tagMode != "and" && tagMode != "or" {
		http.Error(w, `{"error":"tag_mode must be 'and' or 'or'"}`, http.StatusBadRequest)
		return
	}

	items, err := data.ListItemsForProject(r.Context(), h.db, project.ID, data.ItemFilter{
		Kind:     kind,
		Status:   status,
		TagSlugs: tagSlugs,
		TagMode:  tagMode,
	})
	if err != nil {
		log.Printf("ListItemsForProject: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}

	// Batch-fetch tags so item cards can render chips in one round trip.
	itemIDs := make([]string, len(items))
	for i, it := range items {
		itemIDs[i] = it.ID
	}
	tagsByItem, err := data.GetTagsForItems(r.Context(), h.db, itemIDs)
	if err != nil {
		log.Printf("GetTagsForItems: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}
	for i := range items {
		if tags := tagsByItem[items[i].ID]; tags != nil {
			items[i].Tags = tags
		}
	}

	responses, err := h.itemsToResponses(r, items)
	if err != nil {
		log.Printf("itemsToResponses: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(responses); err != nil {
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

	var item *data.Item
	err := data.WithRetry(r.Context(), h.db, func(tx *sql.Tx) error {
		var err error
		item, err = data.CreateItem(r.Context(), tx, project.ID, userID,
			data.ItemKind(body.Kind), body.Title, body.Body)
		if err != nil {
			return err
		}
		_, err = data.LogActivity(r.Context(), tx, data.ActivityInput{
			ProjectID: project.ID,
			ItemID:    &item.ID,
			ActorID:   userID,
			Action:    data.ActivityItemCreated,
			Payload:   map[string]any{"item": itemSnapshot(item)},
		})
		return err
	})
	if err != nil {
		log.Printf("CreateItem: %v", err)
		http.Error(w, `{"error":"Failed to create item"}`, http.StatusInternalServerError)
		return
	}

	// Brand-new item — no tags or attachments yet, but wrap for shape parity.
	resp := ItemResponse{Item: *item, Attachments: []AttachmentResponse{}}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
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

	tags, err := data.GetTagsForItem(r.Context(), h.db, item.ID)
	if err != nil {
		log.Printf("GetTagsForItem: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}
	item.Tags = tags

	resp, err := h.itemToResponse(r, *item)
	if err != nil {
		log.Printf("itemToResponse: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
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

	var updated *data.Item
	err = data.WithRetry(r.Context(), h.db, func(tx *sql.Tx) error {
		var e error
		updated, e = data.UpdateItem(r.Context(), tx, existing.ID, patch)
		if e != nil {
			return e
		}
		diff := itemDiff(existing, updated)
		if len(diff) == 0 {
			return nil // no-op PATCH, do not log an activity
		}
		_, e = data.LogActivity(r.Context(), tx, data.ActivityInput{
			ProjectID: project.ID,
			ItemID:    &updated.ID,
			ActorID:   userID,
			Action:    data.ActivityItemUpdated,
			Payload:   map[string]any{"item": itemSnapshot(updated), "diff": diff},
		})
		return e
	})
	if err != nil {
		log.Printf("UpdateItem: %v", err)
		http.Error(w, `{"error":"Failed to update item"}`, http.StatusInternalServerError)
		return
	}

	tags, err := data.GetTagsForItem(r.Context(), h.db, updated.ID)
	if err != nil {
		log.Printf("GetTagsForItem: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}
	updated.Tags = tags

	resp, err := h.itemToResponse(r, *updated)
	if err != nil {
		log.Printf("itemToResponse: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("encode item: %v", err)
	}
}

// attachTag adds a tag to an item. Accepts {tag_id} | {tag_slug} | {name}
// (precedence in that order). When given a name, performs find-or-create
// against the user's tag namespace (case-insensitive auto-merge).
func (h *ItemsHandler) attachTag(w http.ResponseWriter, r *http.Request, slugOrID string, seq int) {
	project, ok := h.resolveProject(w, r, slugOrID)
	if !ok {
		return
	}
	userID := GetUserID(r.Context())
	if !requireOwner(w, project, userID) {
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

	var body struct {
		TagID   string  `json:"tag_id"`
		TagSlug string  `json:"tag_slug"`
		Name    string  `json:"name"`
		Colour  *string `json:"colour"`
		Icon    *string `json:"icon"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	var tag *data.Tag
	switch {
	case body.TagID != "":
		tag, err = data.GetTagForUser(r.Context(), h.db, userID, body.TagID)
	case body.TagSlug != "":
		tag, err = data.GetTagForUser(r.Context(), h.db, userID, body.TagSlug)
	case strings.TrimSpace(body.Name) != "":
		err = data.WithRetry(r.Context(), h.db, func(tx *sql.Tx) error {
			var e error
			tag, _, e = data.FindOrCreateTag(r.Context(), tx, userID,
				body.Name, "", nil, body.Icon, body.Colour)
			return e
		})
	default:
		http.Error(w, `{"error":"tag_id, tag_slug, or name required"}`, http.StatusBadRequest)
		return
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, `{"error":"Tag not found"}`, http.StatusNotFound)
			return
		}
		log.Printf("resolve tag: %v", err)
		http.Error(w, `{"error":"Failed to resolve tag"}`, http.StatusInternalServerError)
		return
	}

	if err := data.WithRetry(r.Context(), h.db, func(tx *sql.Tx) error {
		if e := data.AttachTagToItem(r.Context(), tx, item.ID, tag.ID, userID); e != nil {
			return e
		}
		_, e := data.LogActivity(r.Context(), tx, data.ActivityInput{
			ProjectID: project.ID,
			ItemID:    &item.ID,
			ActorID:   userID,
			Action:    data.ActivityItemTagAdded,
			Payload:   map[string]any{"item": itemSnapshot(item), "tag": map[string]any{"slug": tag.Slug, "name": tag.Name}},
		})
		return e
	}); err != nil {
		log.Printf("AttachTagToItem: %v", err)
		http.Error(w, `{"error":"Failed to attach tag"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(tag); err != nil {
		log.Printf("encode tag: %v", err)
	}
}

func (h *ItemsHandler) detachTag(w http.ResponseWriter, r *http.Request, slugOrID string, seq int, tagSlug string) {
	project, ok := h.resolveProject(w, r, slugOrID)
	if !ok {
		return
	}
	userID := GetUserID(r.Context())
	if !requireOwner(w, project, userID) {
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

	tag, err := data.GetTagForUser(r.Context(), h.db, userID, tagSlug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, `{"error":"Tag not found"}`, http.StatusNotFound)
			return
		}
		log.Printf("GetTagForUser: %v", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}

	if err := data.WithRetry(r.Context(), h.db, func(tx *sql.Tx) error {
		if e := data.DetachTagFromItem(r.Context(), tx, item.ID, tag.ID); e != nil {
			return e
		}
		_, e := data.LogActivity(r.Context(), tx, data.ActivityInput{
			ProjectID: project.ID,
			ItemID:    &item.ID,
			ActorID:   userID,
			Action:    data.ActivityItemTagRemoved,
			Payload:   map[string]any{"item": itemSnapshot(item), "tag": map[string]any{"slug": tag.Slug, "name": tag.Name}},
		})
		return e
	}); err != nil {
		log.Printf("DetachTagFromItem: %v", err)
		http.Error(w, `{"error":"Failed to detach tag"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
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

	if err := data.WithRetry(r.Context(), h.db, func(tx *sql.Tx) error {
		if _, e := data.LogActivity(r.Context(), tx, data.ActivityInput{
			ProjectID: project.ID,
			ItemID:    &existing.ID,
			ActorID:   userID,
			Action:    data.ActivityItemDeleted,
			Payload:   map[string]any{"item": itemSnapshot(existing)},
		}); e != nil {
			return e
		}
		return data.DeleteItem(r.Context(), tx, existing.ID)
	}); err != nil {
		log.Printf("DeleteItem: %v", err)
		http.Error(w, `{"error":"Failed to delete item"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
