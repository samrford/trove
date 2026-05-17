package data

import (
	"context"
	"database/sql"

	"github.com/lib/pq"
)

// AttachTagToItem links a tag to an item. Idempotent — re-attaching the same
// tag is a silent no-op rather than an error.
func AttachTagToItem(ctx context.Context, tx *sql.Tx, itemID, tagID, taggedBy string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO item_tags (item_id, tag_id, tagged_by)
		VALUES ($1, $2, $3)
		ON CONFLICT (item_id, tag_id) DO NOTHING
	`, itemID, tagID, taggedBy)
	return err
}

// DetachTagFromItem removes a tag from an item. Idempotent — detaching a tag
// that wasn't attached succeeds silently.
func DetachTagFromItem(ctx context.Context, tx *sql.Tx, itemID, tagID string) error {
	_, err := tx.ExecContext(ctx,
		`DELETE FROM item_tags WHERE item_id = $1 AND tag_id = $2`,
		itemID, tagID)
	return err
}

// GetTagsForItem returns all tags currently attached to a single item.
func GetTagsForItem(ctx context.Context, db *sql.DB, itemID string) ([]Tag, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+tagColumns+`
		FROM tags t
		JOIN item_tags it ON it.tag_id = t.id
		WHERE it.item_id = $1
		ORDER BY t.name ASC
	`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tags := []Tag{}
	for rows.Next() {
		tag, err := scanTag(rows)
		if err != nil {
			return nil, err
		}
		tags = append(tags, *tag)
	}
	return tags, rows.Err()
}

// GetTagsForItems batches tag lookups for a list of items. Returns a map keyed
// by item ID. Items with no tags are not present in the map.
func GetTagsForItems(ctx context.Context, db *sql.DB, itemIDs []string) (map[string][]Tag, error) {
	out := map[string][]Tag{}
	if len(itemIDs) == 0 {
		return out, nil
	}

	rows, err := db.QueryContext(ctx, `
		SELECT it.item_id,
		       t.id, t.slug, t.name, t.name_normalised, t.description, t.icon, t.colour,
		       t.user_id, t.group_id, t.archived_at, t.created_at, t.updated_at
		FROM item_tags it
		JOIN tags t ON t.id = it.tag_id
		WHERE it.item_id = ANY($1)
		ORDER BY t.name ASC
	`, pq.Array(itemIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var itemID string
		var t Tag
		if err := rows.Scan(
			&itemID,
			&t.ID, &t.Slug, &t.Name, &t.NameNormalised, &t.Description, &t.Icon, &t.Colour,
			&t.UserID, &t.GroupID, &t.ArchivedAt, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out[itemID] = append(out[itemID], t)
	}
	return out, rows.Err()
}

// ListTagsUsedInProject returns tags that have at least one item in the given
// project, sorted by usage count (within that project) DESC, then name ASC.
// Used by the project-page filter sidebar.
func ListTagsUsedInProject(ctx context.Context, db *sql.DB, projectID string) ([]TagWithCount, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT t.id, t.slug, t.name, t.name_normalised, t.description, t.icon, t.colour,
		       t.user_id, t.group_id, t.archived_at, t.created_at, t.updated_at,
		       COUNT(it.item_id) AS item_count,
		       MAX(it.tagged_at) AS last_used_at
		FROM tags t
		JOIN item_tags it ON it.tag_id = t.id
		JOIN items i      ON i.id = it.item_id
		WHERE i.project_id = $1
		GROUP BY t.id
		ORDER BY item_count DESC, t.name ASC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tags := []TagWithCount{}
	for rows.Next() {
		var t TagWithCount
		if err := rows.Scan(
			&t.ID, &t.Slug, &t.Name, &t.NameNormalised, &t.Description, &t.Icon, &t.Colour,
			&t.UserID, &t.GroupID, &t.ArchivedAt, &t.CreatedAt, &t.UpdatedAt,
			&t.ItemCount, &t.LastUsedAt,
		); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

// ListItemsForTag returns every item the given tag is attached to. Used by the
// /tags/[slug] detail page.
func ListItemsForTag(ctx context.Context, db *sql.DB, tagID string) ([]Item, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+itemColumns+`
		FROM items i
		JOIN item_tags it ON it.item_id = i.id
		WHERE it.tag_id = $1
		ORDER BY i.created_at DESC
	`, tagID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Item{}
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}
