-- Migration 012: Generalize custom fields from user-only to entity-keyed.
--
-- Replaces user_custom_fields (keyed only by user_id) with
-- entity_custom_fields (keyed by entity_type + entity_id) so that custom
-- metadata can be attached to any entity type (e.g. "user", "material",
-- "course") without a separate table per entity.
--
-- Existing user_custom_fields rows are migrated as entity_type='user'.

CREATE TABLE IF NOT EXISTS entity_custom_fields (
    id           INTEGER PRIMARY KEY,
    entity_type  TEXT    NOT NULL,
    entity_id    INTEGER NOT NULL,
    field_name   TEXT    NOT NULL,
    field_value  TEXT,
    is_encrypted INTEGER DEFAULT 0,
    UNIQUE(entity_type, entity_id, field_name)
);

INSERT OR IGNORE INTO entity_custom_fields
    (entity_type, entity_id, field_name, field_value, is_encrypted)
SELECT 'user', user_id, field_name, field_value, is_encrypted
FROM   user_custom_fields;
