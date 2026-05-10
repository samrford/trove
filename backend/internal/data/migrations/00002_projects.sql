-- +goose Up
-- +goose StatementBegin

CREATE TABLE projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    colour TEXT,
    icon TEXT,
    owner_id UUID NOT NULL REFERENCES users(id),
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Slug uniqueness scoped per owner. (When groups land in 00007, the schema
-- gets a nullable group_id and a parallel uniqueness constraint per group.)
CREATE UNIQUE INDEX projects_owner_slug_unique ON projects(owner_id, slug);
CREATE INDEX projects_owner_idx ON projects(owner_id);

-- project_role enumerates roles that go *in project_members*. Owner is via
-- projects.owner_id (single source of truth, not duplicated here).
CREATE TYPE project_role AS ENUM ('contributor', 'viewer');

-- Per-project ad-hoc shares (also used to override a group member's role
-- on a specific project once groups exist).
CREATE TABLE project_members (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    role project_role NOT NULL,
    invited_by UUID REFERENCES users(id),
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, project_id)
);

CREATE INDEX project_members_project_idx ON project_members(project_id);

-- Atomic counter for per-project monotonic item sequences (the `#42` in trove#42).
-- Claim next via: UPDATE ... SET next_sequence = next_sequence + 1 RETURNING next_sequence - 1.
CREATE TABLE project_item_sequences (
    project_id UUID PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    next_sequence INT NOT NULL DEFAULT 1
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS project_item_sequences;
DROP TABLE IF EXISTS project_members;
DROP TABLE IF EXISTS projects;
DROP TYPE IF EXISTS project_role;

-- +goose StatementEnd
