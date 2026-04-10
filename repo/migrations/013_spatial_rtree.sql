-- 013_spatial_rtree.sql
-- Adds an R*Tree virtual table on the locations table to support fast
-- bounding-box pre-filtering before Point-in-Polygon containment tests.
--
-- Coordinate convention:
--   minX / maxX  →  longitude range
--   minY / maxY  →  latitude range
--
-- For point locations (lat/lng present), minX=maxX=lng and minY=maxY=lat.
-- For polygon regions, the application populates the bounding box from the
-- WKT geometry when calling ComputeRegionAggregation.

CREATE VIRTUAL TABLE IF NOT EXISTS location_rtree USING rtree(
    id,
    minX, maxX,
    minY, maxY
);

-- ---------------------------------------------------------------
-- Triggers: keep location_rtree in sync with locations table.
-- ---------------------------------------------------------------

CREATE TRIGGER IF NOT EXISTS location_rtree_ai
AFTER INSERT ON locations
WHEN new.lat IS NOT NULL AND new.lng IS NOT NULL
BEGIN
    INSERT OR IGNORE INTO location_rtree (id, minX, maxX, minY, maxY)
    VALUES (new.id, new.lng, new.lng, new.lat, new.lat);
END;

CREATE TRIGGER IF NOT EXISTS location_rtree_au
AFTER UPDATE OF lat, lng ON locations
BEGIN
    DELETE FROM location_rtree WHERE id = old.id;
    INSERT OR IGNORE INTO location_rtree (id, minX, maxX, minY, maxY)
    SELECT new.id, new.lng, new.lng, new.lat, new.lat
    WHERE  new.lat IS NOT NULL AND new.lng IS NOT NULL;
END;

CREATE TRIGGER IF NOT EXISTS location_rtree_ad
AFTER DELETE ON locations
BEGIN
    DELETE FROM location_rtree WHERE id = old.id;
END;
