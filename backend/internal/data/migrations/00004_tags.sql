-- +goose Up
-- +goose StatementBegin

CREATE TABLE tags (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug TEXT NOT NULL,
    name TEXT NOT NULL,
    -- Lowercased+trimmed copy of `name`, used for case-insensitive uniqueness
    -- and the auto-merge lookup ("Bug" == "bug" == "  BUG  ").
    name_normalised TEXT NOT NULL,
    description TEXT,
    icon TEXT,
    colour TEXT,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    -- Reserved for groups (00007). Today: always NULL; the CHECK below enforces
    -- user-scoped. When groups land, the check tightens into a XOR.
    group_id UUID,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (user_id IS NOT NULL)
);

-- Slug + normalised-name uniqueness scoped per owner. (When groups land in
-- 00007, parallel constraints scoped per group_id are added.)
CREATE UNIQUE INDEX tags_owner_slug_unique       ON tags(user_id, slug);
CREATE UNIQUE INDEX tags_owner_normalised_unique ON tags(user_id, name_normalised);

CREATE TABLE item_tags (
    item_id   UUID NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    tag_id    UUID NOT NULL REFERENCES tags(id)  ON DELETE CASCADE,
    tagged_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    tagged_by UUID NOT NULL REFERENCES users(id),
    PRIMARY KEY (item_id, tag_id)
);

CREATE INDEX item_tags_tag_idx ON item_tags(tag_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS item_tags;
DROP TABLE IF EXISTS tags;

-- +goose StatementEnd
