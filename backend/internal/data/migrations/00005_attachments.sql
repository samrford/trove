-- +goose Up
-- +goose StatementBegin

-- Two ways an attachment can land in Trove: a direct upload from the user, or
-- pulled in via the Google Photos picker. The source field exists so future
-- features (e.g. "re-import from Google Photos") can branch on origin without
-- inspecting the storage key.
CREATE TYPE attachment_source AS ENUM ('upload', 'google_photos');

CREATE TABLE attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    -- Nullable: future iteration will allow project-level files. Today every
    -- attachment is item-scoped so the handler always sets this.
    item_id UUID REFERENCES items(id) ON DELETE CASCADE,
    -- Object key in Tigris/MinIO. Format: projects/{project_id}/items/{item_id}/{uuid}{ext}
    storage_key TEXT NOT NULL UNIQUE,
    filename TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL,
    source attachment_source NOT NULL DEFAULT 'upload',
    uploader_id UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX attachments_item_idx ON attachments(item_id) WHERE item_id IS NOT NULL;
CREATE INDEX attachments_project_idx ON attachments(project_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS attachments;
DROP TYPE IF EXISTS attachment_source;

-- +goose StatementEnd
