-- +goose Up
-- +goose StatementBegin

-- SSE fan-out substrate. Every committed activity row emits a NOTIFY *by
-- construction* — the DB-level analogue of the *sql.Tx write-path guardrail:
-- impossible to forget at a call site, future emit sites (MCP `log`) get it
-- free. Postgres delivers NOTIFY only on COMMIT, so a rolled-back tx or a
-- no-op PATCH (which never INSERTs) correctly produces no event with zero
-- special handling.
--
-- The payload is a SLIM envelope, never the full activity row: pg_notify caps
-- at 8000 bytes and body-diff payloads can exceed that. The hub re-reads the
-- full row (and the live item/project) by id — an indexed PK lookup.
CREATE FUNCTION trove_notify_activity() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('trove_events', json_build_object(
        'activity_id', NEW.id,
        'project_id',  NEW.project_id,
        'item_id',     NEW.item_id,
        'action',      NEW.action,
        'created_at',  to_char(NEW.created_at AT TIME ZONE 'UTC',
                               'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
    )::text);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER activity_notify
    AFTER INSERT ON activity
    FOR EACH ROW EXECUTE FUNCTION trove_notify_activity();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TRIGGER IF EXISTS activity_notify ON activity;
DROP FUNCTION IF EXISTS trove_notify_activity();

-- +goose StatementEnd
