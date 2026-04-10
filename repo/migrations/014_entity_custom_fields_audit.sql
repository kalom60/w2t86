-- 014_entity_custom_fields_audit.sql
-- Immutable audit trail for every mutation on entity_custom_fields.
-- Captures who changed what, when, and why for every set/delete operation.
-- Rows in this table are never updated or deleted.

CREATE TABLE IF NOT EXISTS entity_custom_fields_audit (
    id           INTEGER PRIMARY KEY,
    entity_type  TEXT    NOT NULL,
    entity_id    INTEGER NOT NULL,
    field_name   TEXT    NOT NULL,
    old_value    TEXT,           -- NULL when the field did not previously exist
    new_value    TEXT,           -- NULL when the field is deleted
    is_encrypted INTEGER DEFAULT 0,
    actor_id     INTEGER NOT NULL REFERENCES users(id),
    reason       TEXT    NOT NULL,
    changed_at   TEXT    DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_ecf_audit_entity
ON entity_custom_fields_audit(entity_type, entity_id);
