-- +goose Up
-- +goose StatementBegin

-- Promote a brainstorm → task by updating `kind` in place; no row move
-- required. (DECISIONS.md, "Data model v1".)
CREATE TYPE item_kind AS ENUM ('brainstorm', 'task');

-- Single status enum across all kinds; the UI relabels per-kind on action
-- buttons. Section headers use the canonical label.
CREATE TYPE item_status AS ENUM ('open', 'in_progress', 'done', 'archived');

CREATE TABLE items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    sequence INT NOT NULL,
    kind item_kind NOT NULL,
    status item_status NOT NULL DEFAULT 'open',
    title TEXT NOT NULL,
    body TEXT,
    -- Fractional position for ordering within a shelf. Seeded with creation
    -- epoch so newest items appear first by default; manual drag-reorder
    -- writes a midpoint between neighbours.
    position DOUBLE PRECISION NOT NULL DEFAULT (EXTRACT(EPOCH FROM NOW()) * 1000),
    creator_id UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Full-text search vector. Title weighted higher than body so a title
    -- hit ranks above an incidental body hit.
    search_vector tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('english', title), 'A') ||
        setweight(to_tsvector('english', coalesce(body, '')), 'B')
    ) STORED
);

CREATE UNIQUE INDEX items_project_sequence_unique ON items(project_id, sequence);
CREATE INDEX items_project_status_idx ON items(project_id, status);
CREATE INDEX items_search_idx ON items USING GIN (search_vector);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS items;
DROP TYPE IF EXISTS item_status;
DROP TYPE IF EXISTS item_kind;

-- +goose StatementEnd
