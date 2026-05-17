-- +goose Up
-- +goose StatementBegin

-- The activity log is two things wearing one hat: a human-facing feed and the
-- complete event substrate the AI reads via MCP / Context Pack. So the data
-- layer records every mutation at full fidelity (every changed field incl.
-- position, actor always, self-contained payloads). Display surfaces decide
-- what's noise for humans — the data stays whole regardless.
CREATE TYPE activity_action AS ENUM (
    'item.created', 'item.updated', 'item.deleted',
    'item.tag_added', 'item.tag_removed',
    'attachment.added', 'attachment.removed',
    'project.created', 'project.updated', 'project.deleted',
    -- Reserved: actor-authored free-text note. No endpoint yet; lands with the
    -- MCP `log` tool. Declared now so the enum doesn't need a later migration.
    'note'
);

CREATE TABLE activity (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    -- SET NULL, not CASCADE: deleting an item keeps its history (incl. the
    -- final item.deleted event). The payload snapshots {seq,title,kind} so a
    -- detached row still reads for the feed and the AI.
    item_id    UUID REFERENCES items(id) ON DELETE SET NULL,
    actor_id   UUID NOT NULL REFERENCES users(id),
    action     activity_action NOT NULL,
    payload    JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Project timeline (feed + keyset pagination), newest-first. The (created_at,
-- id) tail matches the ListActivity cursor's row-value comparison.
CREATE INDEX activity_project_created_idx
    ON activity (project_id, created_at DESC, id DESC);

-- Per-item rail + the "filter to one item" case on the rich page.
CREATE INDEX activity_item_created_idx
    ON activity (item_id, created_at DESC, id DESC) WHERE item_id IS NOT NULL;

-- Actor filter on the rich page.
CREATE INDEX activity_actor_idx ON activity (actor_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS activity;
DROP TYPE IF EXISTS activity_action;

-- +goose StatementEnd
