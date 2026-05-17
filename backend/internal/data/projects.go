package data

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// ErrSlugTaken is returned when a project create/rename conflicts with an
// existing slug for the same owner.
var ErrSlugTaken = errors.New("slug already taken")

type Project struct {
	ID          string     `json:"id"`
	Slug        string     `json:"slug"`
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	Colour      *string    `json:"colour"`
	Icon        *string    `json:"icon"`
	OwnerID     string     `json:"owner_id"`
	ArchivedAt  *time.Time `json:"archived_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// IsValidSlug returns true iff the slug is in canonical form (non-empty,
// lowercase alphanumerics, single dashes between, no leading/trailing dashes).
// The frontend mirrors this via $lib/slug.ts.
func IsValidSlug(slug string) bool {
	return slug != "" && slug == generateSlug(slug)
}

// SlugExistsForOwner reports whether the given owner already has a project with
// this slug (active or archived — slugs are unique per owner_id at the DB level).
func SlugExistsForOwner(ctx context.Context, db *sql.DB, ownerID, slug string) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM projects WHERE owner_id = $1 AND slug = $2)
	`, ownerID, slug).Scan(&exists)
	return exists, err
}

// generateSlug turns "My Project!" into "my-project". Returns "" if the input
// has no alphanumeric characters — caller decides what to do.
func generateSlug(name string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash && b.Len() > 0 {
			b.WriteRune('-')
			prevDash = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// CreateProject inserts a project + its initial sequence row. The two
// statements must run in one transaction (call inside WithRetry) so a project
// can't exist without its sequence row and a same-unit activity row commits
// with it. If slug is empty, it's auto-derived from name.
func CreateProject(ctx context.Context, tx *sql.Tx, ownerID, slug, name string, description, colour, icon *string) (*Project, error) {
	if slug == "" {
		slug = generateSlug(name)
	}
	if slug == "" {
		return nil, errors.New("project name must contain at least one alphanumeric character")
	}

	var p Project
	err := tx.QueryRowContext(ctx, `
		INSERT INTO projects (slug, name, description, colour, icon, owner_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, slug, name, description, colour, icon, owner_id, archived_at, created_at, updated_at
	`, slug, name, description, colour, icon, ownerID).Scan(
		&p.ID, &p.Slug, &p.Name, &p.Description, &p.Colour, &p.Icon,
		&p.OwnerID, &p.ArchivedAt, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return nil, ErrSlugTaken
		}
		return nil, err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO project_item_sequences (project_id, next_sequence) VALUES ($1, 1)
	`, p.ID); err != nil {
		return nil, err
	}

	return &p, nil
}

// ListProjectsForUser returns all non-archived projects the user owns or is an
// explicit member of, ordered by most recently updated.
func ListProjectsForUser(ctx context.Context, db *sql.DB, userID string) ([]Project, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT p.id, p.slug, p.name, p.description, p.colour, p.icon,
		                p.owner_id, p.archived_at, p.created_at, p.updated_at
		FROM projects p
		LEFT JOIN project_members pm ON pm.project_id = p.id AND pm.user_id = $1
		WHERE (p.owner_id = $1 OR pm.user_id IS NOT NULL)
		  AND p.archived_at IS NULL
		ORDER BY p.updated_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	projects := []Project{}
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Slug, &p.Name, &p.Description, &p.Colour,
			&p.Icon, &p.OwnerID, &p.ArchivedAt, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// UpdateProject overwrites the editable fields on a project. If slug is
// non-empty it's also updated; pass "" to leave it unchanged.
func UpdateProject(ctx context.Context, tx *sql.Tx, projectID, name, slug string, description, colour, icon *string) (*Project, error) {
	var p Project
	err := tx.QueryRowContext(ctx, `
		UPDATE projects
		SET name = $1,
		    slug = COALESCE(NULLIF($2, ''), slug),
		    description = $3,
		    colour = $4,
		    icon = $5,
		    updated_at = NOW()
		WHERE id = $6
		RETURNING id, slug, name, description, colour, icon, owner_id, archived_at, created_at, updated_at
	`, name, slug, description, colour, icon, projectID).Scan(
		&p.ID, &p.Slug, &p.Name, &p.Description, &p.Colour, &p.Icon,
		&p.OwnerID, &p.ArchivedAt, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return nil, ErrSlugTaken
		}
		return nil, err
	}
	return &p, nil
}

// DeleteProject removes a project and all dependent rows (members, sequences,
// future items/tags/etc.) via ON DELETE CASCADE on every FK pointing at it.
func DeleteProject(ctx context.Context, tx *sql.Tx, projectID string) error {
	res, err := tx.ExecContext(ctx, `DELETE FROM projects WHERE id = $1`, projectID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GetProjectForUser fetches a single project by slug or UUID, ensuring the user
// has access (owner or member). Returns sql.ErrNoRows if not found / unauthorised.
func GetProjectForUser(ctx context.Context, db *sql.DB, userID, slugOrID string) (*Project, error) {
	matchCol := "p.slug"
	if _, err := uuid.Parse(slugOrID); err == nil {
		matchCol = "p.id"
	}

	var p Project
	err := db.QueryRowContext(ctx, `
		SELECT p.id, p.slug, p.name, p.description, p.colour, p.icon,
		       p.owner_id, p.archived_at, p.created_at, p.updated_at
		FROM projects p
		LEFT JOIN project_members pm ON pm.project_id = p.id AND pm.user_id = $1
		WHERE `+matchCol+` = $2 AND (p.owner_id = $1 OR pm.user_id IS NOT NULL)
		LIMIT 1
	`, userID, slugOrID).Scan(
		&p.ID, &p.Slug, &p.Name, &p.Description, &p.Colour, &p.Icon,
		&p.OwnerID, &p.ArchivedAt, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

