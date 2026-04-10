-- 016_material_price.sql
-- Adds an authoritative unit price to the materials (catalog) table.
-- All order pricing is now computed server-side from this column,
-- eliminating the client-supplied unit_price trust boundary.

ALTER TABLE materials ADD COLUMN price REAL NOT NULL DEFAULT 0;
