-- +goose Up
-- +goose StatementBegin

-- Trove-side mapping for the google-photos-picker library: tells the sink
-- (and any future feature that branches on origin) which item a given import
-- job is destined for. The picker library has its own tables for tokens and
-- import-job state — those are owned by ppg.Migrate(db) at startup, not here.
CREATE TABLE gphotos_import_targets (
    job_id TEXT PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    item_id UUID NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX gphotos_import_targets_item_idx ON gphotos_import_targets(item_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS gphotos_import_targets;

-- +goose StatementEnd
