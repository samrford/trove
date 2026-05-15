package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// ErrTagSlugTaken is returned when a tag create/rename hits an existing slug
// for the same owner. (Distinct from project slug collisions.)
var ErrTagSlugTaken = errors.New("tag slug already taken")

type Tag struct {
	ID             string     `json:"id"`
	Slug           string     `json:"slug"`
	Name           string     `json:"name"`
	NameNormalised string     `json:"-"`
	Description    *string    `json:"description"`
	Icon           *string    `json:"icon"`
	Colour         *string    `json:"colour"`
	UserID         *string    `json:"user_id"`
	GroupID        *string    `json:"group_id"`
	ArchivedAt     *time.Time `json:"archived_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// TagWithCount is a Tag plus a usage count and the most-recent application
// timestamp. Used by list views — the frontend picks its own sort order.
type TagWithCount struct {
	Tag
	ItemCount  int        `json:"item_count"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

const tagColumns = `id, slug, name, name_normalised, description, icon, colour,
	user_id, group_id, archived_at, created_at, updated_at`

func scanTag(row interface {
	Scan(dest ...any) error
}) (*Tag, error) {
	var t Tag
	if err := row.Scan(
		&t.ID, &t.Slug, &t.Name, &t.NameNormalised, &t.Description, &t.Icon, &t.Colour,
		&t.UserID, &t.GroupID, &t.ArchivedAt, &t.CreatedAt, &t.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &t, nil
}

// normaliseTagName collapses casing/whitespace so "Bug", "bug", "  BUG  " all
// resolve to the same canonical key. Used for the auto-merge lookup.
func normaliseTagName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// TagSlugExistsForOwner reports whether the given owner already has a tag with
// this slug. Mirrors SlugExistsForOwner for projects.
func TagSlugExistsForOwner(ctx context.Context, db *sql.DB, ownerID, slug string) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM tags WHERE user_id = $1 AND slug = $2)
	`, ownerID, slug).Scan(&exists)
	return exists, err
}

// FindOrCreateTag implements the auto-merge create semantic. If a tag with the
// same normalised name already exists for this user, returns it (created=false).
// Otherwise inserts a new tag (created=true). Slug is auto-generated from name
// when blank; an explicit slug that collides with a *different* tag returns
// ErrTagSlugTaken.
func FindOrCreateTag(ctx context.Context, db *sql.DB, userID, name, slug string, description, icon, colour *string) (*Tag, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, false, errors.New("tag name is required")
	}
	normalised := normaliseTagName(name)

	if slug == "" {
		slug = generateSlug(name)
	}
	if slug == "" {
		return nil, false, errors.New("tag name must contain at least one alphanumeric character")
	}

	row := db.QueryRowContext(ctx, `
		INSERT INTO tags (slug, name, name_normalised, description, icon, colour, user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+tagColumns,
		slug, name, normalised, description, icon, colour, userID)

	tag, err := scanTag(row)
	if err == nil {
		return tag, true, nil
	}

	pqErr, ok := err.(*pq.Error)
	if !ok || pqErr.Code != PGUniqueViolation {
		return nil, false, err
	}

	// Auto-merge: normalised name already exists → return the existing tag.
	if pqErr.Constraint == ConstraintTagsOwnerNormalisedUnique {
		existing, err := getTagByNormalisedName(ctx, db, userID, normalised)
		return existing, false, err
	}
	// Slug collision with a *different* tag.
	if pqErr.Constraint == ConstraintTagsOwnerSlugUnique {
		return nil, false, ErrTagSlugTaken
	}
	return nil, false, err
}

func getTagByNormalisedName(ctx context.Context, db *sql.DB, userID, normalised string) (*Tag, error) {
	row := db.QueryRowContext(ctx,
		`SELECT `+tagColumns+` FROM tags WHERE user_id = $1 AND name_normalised = $2`,
		userID, normalised)
	return scanTag(row)
}

// GetTagForUser fetches a single tag by slug or UUID, scoped to the user.
// Returns sql.ErrNoRows if not found.
func GetTagForUser(ctx context.Context, db *sql.DB, userID, slugOrID string) (*Tag, error) {
	matchCol := "slug"
	if _, err := uuid.Parse(slugOrID); err == nil {
		matchCol = "id"
	}
	row := db.QueryRowContext(ctx,
		`SELECT `+tagColumns+` FROM tags WHERE user_id = $1 AND `+matchCol+` = $2`,
		userID, slugOrID)
	return scanTag(row)
}

// ListTagsForUser returns the user's tags joined with usage counts and
// last-used timestamps, sorted by item count DESC (alphabetical tiebreaker).
// Frontend re-sorts client-side for combobox-style "recently used" views.
func ListTagsForUser(ctx context.Context, db *sql.DB, userID string) ([]TagWithCount, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT t.id, t.slug, t.name, t.name_normalised, t.description, t.icon, t.colour,
		       t.user_id, t.group_id, t.archived_at, t.created_at, t.updated_at,
		       COALESCE(c.cnt, 0) AS item_count,
		       c.last_used_at
		FROM tags t
		LEFT JOIN (
			SELECT tag_id, COUNT(*) AS cnt, MAX(tagged_at) AS last_used_at
			FROM item_tags GROUP BY tag_id
		) c ON c.tag_id = t.id
		WHERE t.user_id = $1
		ORDER BY item_count DESC, t.name ASC
	`, userID)
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

// UpdateTag overwrites the editable fields. If slug is non-empty, it's also
// updated; pass "" to leave it unchanged.
func UpdateTag(ctx context.Context, db *sql.DB, tagID, name, slug string, description, icon, colour *string) (*Tag, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("tag name is required")
	}
	normalised := normaliseTagName(name)

	row := db.QueryRowContext(ctx, `
		UPDATE tags
		SET name = $1,
		    name_normalised = $2,
		    slug = COALESCE(NULLIF($3, ''), slug),
		    description = $4,
		    icon = $5,
		    colour = $6,
		    updated_at = NOW()
		WHERE id = $7
		RETURNING `+tagColumns,
		name, normalised, slug, description, icon, colour, tagID)

	tag, err := scanTag(row)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == PGUniqueViolation {
			if pqErr.Constraint == ConstraintTagsOwnerSlugUnique {
				return nil, ErrTagSlugTaken
			}
			// Renaming into an existing normalised name → conflict. We don't
			// auto-merge on update (would silently delete the renamed tag).
			if pqErr.Constraint == ConstraintTagsOwnerNormalisedUnique {
				return nil, fmt.Errorf("a tag with that name already exists")
			}
		}
		return nil, err
	}
	return tag, nil
}

// DeleteTag removes a tag. item_tags rows cascade.
func DeleteTag(ctx context.Context, db *sql.DB, tagID string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM tags WHERE id = $1`, tagID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}
