package repository

import (
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"

	"w2t86/internal/models"
)

// AnalyticsRepository provides all KPI, spatial, and export queries.
type AnalyticsRepository struct {
	db *sql.DB
}

// NewAnalyticsRepository returns an AnalyticsRepository backed by the given database.
func NewAnalyticsRepository(db *sql.DB) *AnalyticsRepository {
	return &AnalyticsRepository{db: db}
}

// InventoryLevel summarises stock levels for one material.
type InventoryLevel struct {
	MaterialID   int64
	Title        string
	TotalQty     int
	AvailableQty int
	ReservedQty  int
}

// MaterialStat carries order-count and average-rating for a material.
type MaterialStat struct {
	MaterialID int64
	Title      string
	OrderCount int
	AvgRating  float64
}

// CourseOrderStat holds per-section/material demand and fulfillment data for
// an instructor, derived from the actual course_plans, course_sections, and
// order_items tables.
type CourseOrderStat struct {
	CourseName    string
	SectionName   string // empty when no section is assigned
	MaterialTitle string
	RequestedQty  int
	ApprovedQty   int
	PlanStatus    string
	Ordered       int // order_items rows that reference this material
	Fulfilled     int // order_items with fulfillment_status='fulfilled'
}

// OrderExportRow is a flattened row for the orders CSV export.
type OrderExportRow struct {
	OrderID     int64
	UserName    string
	UserEmail   string
	Status      string
	TotalAmount float64
	CreatedAt   string
	ItemCount   int
}

// DistribExportRow is a flattened row for the distribution-events CSV export.
type DistribExportRow struct {
	ScanID        string
	EventType     string
	MaterialTitle string
	ActorName     string
	OccurredAt    string
}

// ---------------------------------------------------------------
// KPI queries
// ---------------------------------------------------------------

// OrdersByStatus returns a map of order-status → count for all non-deleted orders.
func (r *AnalyticsRepository) OrdersByStatus() (map[string]int, error) {
	const q = `
		SELECT status, COUNT(*) AS cnt
		FROM   orders
		GROUP  BY status`

	rows, err := r.db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("analytics: OrdersByStatus: %w", err)
	}
	defer rows.Close()

	out := make(map[string]int)
	for rows.Next() {
		var status string
		var cnt int
		if err := rows.Scan(&status, &cnt); err != nil {
			return nil, fmt.Errorf("analytics: OrdersByStatus scan: %w", err)
		}
		out[status] = cnt
	}
	return out, rows.Err()
}

// periodExpr converts a period string ("7d", "30d", "90d") to a SQLite
// datetime modifier.  Defaults to 30 days.
func periodExpr(period string) string {
	switch period {
	case "7d":
		return "-7 days"
	case "90d":
		return "-90 days"
	default:
		return "-30 days"
	}
}

// FulfillmentRate returns the fraction of orders (in the given period) whose
// status is "completed", expressed as a value 0–100.
func (r *AnalyticsRepository) FulfillmentRate(period string) (float64, error) {
	mod := periodExpr(period)
	const q = `
		SELECT
			CAST(SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) AS REAL) /
			NULLIF(COUNT(*), 0) * 100
		FROM orders
		WHERE created_at >= datetime('now', ?)`

	var rate sql.NullFloat64
	if err := r.db.QueryRow(q, mod).Scan(&rate); err != nil {
		return 0, fmt.Errorf("analytics: FulfillmentRate: %w", err)
	}
	if !rate.Valid {
		return 0, nil
	}
	return rate.Float64, nil
}

// ReturnRate returns the fraction of completed orders (in the given period)
// that have at least one return request, expressed as a value 0–100.
func (r *AnalyticsRepository) ReturnRate(period string) (float64, error) {
	mod := periodExpr(period)
	const q = `
		SELECT
			CAST(COUNT(DISTINCT rr.order_id) AS REAL) /
			NULLIF(COUNT(DISTINCT o.id), 0) * 100
		FROM   orders o
		LEFT   JOIN return_requests rr ON rr.order_id = o.id
		WHERE  o.status = 'completed'
		  AND  o.created_at >= datetime('now', ?)`

	var rate sql.NullFloat64
	if err := r.db.QueryRow(q, mod).Scan(&rate); err != nil {
		return 0, fmt.Errorf("analytics: ReturnRate: %w", err)
	}
	if !rate.Valid {
		return 0, nil
	}
	return rate.Float64, nil
}

// InventoryLevels returns stock levels for all non-deleted, active materials.
func (r *AnalyticsRepository) InventoryLevels() ([]InventoryLevel, error) {
	const q = `
		SELECT id, title, total_qty, available_qty, reserved_qty
		FROM   materials
		WHERE  deleted_at IS NULL
		ORDER  BY title`

	rows, err := r.db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("analytics: InventoryLevels: %w", err)
	}
	defer rows.Close()

	var out []InventoryLevel
	for rows.Next() {
		var l InventoryLevel
		if err := rows.Scan(&l.MaterialID, &l.Title, &l.TotalQty, &l.AvailableQty, &l.ReservedQty); err != nil {
			return nil, fmt.Errorf("analytics: InventoryLevels scan: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ActiveUserCount returns the number of users that have placed at least one
// order in the last 30 days.
func (r *AnalyticsRepository) ActiveUserCount() (int, error) {
	const q = `
		SELECT COUNT(DISTINCT user_id)
		FROM   orders
		WHERE  created_at >= datetime('now', '-30 days')`

	var n int
	if err := r.db.QueryRow(q).Scan(&n); err != nil {
		return 0, fmt.Errorf("analytics: ActiveUserCount: %w", err)
	}
	return n, nil
}

// TopMaterials returns the top N materials by total order-item count, with
// their average star rating.
func (r *AnalyticsRepository) TopMaterials(limit int) ([]MaterialStat, error) {
	const q = `
		SELECT m.id,
		       m.title,
		       COUNT(oi.id)                                    AS order_count,
		       COALESCE(AVG(CAST(rt.stars AS REAL)), 0.0)     AS avg_rating
		FROM   materials m
		JOIN   order_items oi ON oi.material_id = m.id
		LEFT   JOIN ratings rt ON rt.material_id = m.id
		WHERE  m.deleted_at IS NULL
		GROUP  BY m.id, m.title
		ORDER  BY order_count DESC
		LIMIT  ?`

	rows, err := r.db.Query(q, limit)
	if err != nil {
		return nil, fmt.Errorf("analytics: TopMaterials: %w", err)
	}
	defer rows.Close()

	var out []MaterialStat
	for rows.Next() {
		var s MaterialStat
		if err := rows.Scan(&s.MaterialID, &s.Title, &s.OrderCount, &s.AvgRating); err != nil {
			return nil, fmt.Errorf("analytics: TopMaterials scan: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------
// KPI snapshots
// ---------------------------------------------------------------

// SaveKPISnapshot inserts a new kpi_snapshots row.
func (r *AnalyticsRepository) SaveKPISnapshot(name, dimension string, value float64, period string) error {
	const q = `
		INSERT INTO kpi_snapshots (metric_name, dimension, value, period, computed_at)
		VALUES (?, ?, ?, ?, datetime('now'))`

	var dimPtr *string
	if dimension != "" {
		dimPtr = &dimension
	}
	var periodPtr *string
	if period != "" {
		periodPtr = &period
	}

	if _, err := r.db.Exec(q, name, dimPtr, value, periodPtr); err != nil {
		return fmt.Errorf("analytics: SaveKPISnapshot: %w", err)
	}
	return nil
}

// GetKPIHistory returns the most-recent `limit` snapshots for the given metric
// and dimension, ordered newest-first.
func (r *AnalyticsRepository) GetKPIHistory(name, dimension string, limit int) ([]models.KPISnapshot, error) {
	const q = `
		SELECT id, metric_name, dimension, value, period, computed_at
		FROM   kpi_snapshots
		WHERE  metric_name = ?
		  AND  (dimension = ? OR (dimension IS NULL AND ? = ''))
		ORDER  BY computed_at DESC
		LIMIT  ?`

	rows, err := r.db.Query(q, name, dimension, dimension, limit)
	if err != nil {
		return nil, fmt.Errorf("analytics: GetKPIHistory: %w", err)
	}
	defer rows.Close()

	var out []models.KPISnapshot
	for rows.Next() {
		var k models.KPISnapshot
		if err := rows.Scan(&k.ID, &k.MetricName, &k.Dimension, &k.Value, &k.Period, &k.ComputedAt); err != nil {
			return nil, fmt.Errorf("analytics: GetKPIHistory scan: %w", err)
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------
// Instructor: course order stats
// ---------------------------------------------------------------

// CourseOrderStats returns per-section/material demand and fulfillment data
// for all course plans owned by the given instructor.  Results come from the
// actual course_plans, course_sections, courses, and order_items tables — no
// approximations.
func (r *AnalyticsRepository) CourseOrderStats(instructorID int64) ([]CourseOrderStat, error) {
	const q = `
		SELECT
			c.name                                                                AS course_name,
			COALESCE(cs.name, '')                                                 AS section_name,
			m.title                                                               AS material_title,
			cp.requested_qty,
			cp.approved_qty,
			cp.status                                                             AS plan_status,
			COUNT(oi.id)                                                          AS ordered,
			SUM(CASE WHEN oi.fulfillment_status = 'fulfilled' THEN 1 ELSE 0 END) AS fulfilled
		FROM   courses c
		JOIN   course_plans cp ON cp.course_id = c.id
		LEFT   JOIN course_sections cs ON cs.id = cp.section_id
		JOIN   materials m ON m.id = cp.material_id
		LEFT   JOIN order_items oi ON oi.material_id = m.id
		LEFT   JOIN orders o ON o.id = oi.order_id AND o.status != 'canceled'
		WHERE  c.instructor_id = ?
		  AND  m.deleted_at IS NULL
		GROUP  BY c.id, COALESCE(cp.section_id, 0), cp.material_id
		ORDER  BY c.name, COALESCE(cs.name, ''), m.title`

	rows, err := r.db.Query(q, instructorID)
	if err != nil {
		return nil, fmt.Errorf("analytics: CourseOrderStats: %w", err)
	}
	defer rows.Close()

	var out []CourseOrderStat
	for rows.Next() {
		var s CourseOrderStat
		if err := rows.Scan(&s.CourseName, &s.SectionName, &s.MaterialTitle,
			&s.RequestedQty, &s.ApprovedQty, &s.PlanStatus,
			&s.Ordered, &s.Fulfilled); err != nil {
			return nil, fmt.Errorf("analytics: CourseOrderStats scan: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// CountPendingPlanItems returns the number of course_plans rows with
// status='pending' across all courses owned by the given instructor.
func (r *AnalyticsRepository) CountPendingPlanItems(instructorID int64) (int, error) {
	const q = `
		SELECT COUNT(*)
		FROM   course_plans cp
		JOIN   courses c ON c.id = cp.course_id
		WHERE  c.instructor_id = ? AND cp.status = 'pending'`
	var n int
	if err := r.db.QueryRow(q, instructorID).Scan(&n); err != nil {
		return 0, fmt.Errorf("analytics: CountPendingPlanItems: %w", err)
	}
	return n, nil
}

// ---------------------------------------------------------------
// Spatial queries
// ---------------------------------------------------------------

// GetLocations returns all locations of the given type.  Pass "" to return all.
func (r *AnalyticsRepository) GetLocations(locType string) ([]models.Location, error) {
	q := `SELECT id, name, type, geom_wkt, lat, lng, properties, created_at FROM locations`
	args := []interface{}{}
	if locType != "" {
		q += ` WHERE type = ?`
		args = append(args, locType)
	}
	q += ` ORDER BY name`

	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("analytics: GetLocations: %w", err)
	}
	defer rows.Close()

	var out []models.Location
	for rows.Next() {
		var l models.Location
		if err := rows.Scan(&l.ID, &l.Name, &l.Type, &l.GeomWKT, &l.Lat, &l.Lng, &l.Properties, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("analytics: GetLocations scan: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// GetSpatialAggregates returns all spatial_aggregates rows for the given layer type.
func (r *AnalyticsRepository) GetSpatialAggregates(layerType string) ([]models.SpatialAggregate, error) {
	const q = `
		SELECT id, layer_type, cell_key, metric, value, computed_at
		FROM   spatial_aggregates
		WHERE  layer_type = ?
		ORDER  BY cell_key`

	rows, err := r.db.Query(q, layerType)
	if err != nil {
		return nil, fmt.Errorf("analytics: GetSpatialAggregates: %w", err)
	}
	defer rows.Close()

	var out []models.SpatialAggregate
	for rows.Next() {
		var sa models.SpatialAggregate
		if err := rows.Scan(&sa.ID, &sa.LayerType, &sa.CellKey, &sa.Metric, &sa.Value, &sa.ComputedAt); err != nil {
			return nil, fmt.Errorf("analytics: GetSpatialAggregates scan: %w", err)
		}
		out = append(out, sa)
	}
	return out, rows.Err()
}

// UpsertSpatialAggregate inserts or replaces a spatial_aggregates row.
func (r *AnalyticsRepository) UpsertSpatialAggregate(layerType, cellKey, metric string, value float64) error {
	const q = `
		INSERT INTO spatial_aggregates (layer_type, cell_key, metric, value, computed_at)
		VALUES (?, ?, ?, ?, datetime('now'))
		ON CONFLICT(layer_type, cell_key, metric)
		DO UPDATE SET value = excluded.value, computed_at = excluded.computed_at`

	if _, err := r.db.Exec(q, layerType, cellKey, metric, value); err != nil {
		return fmt.Errorf("analytics: UpsertSpatialAggregate: %w", err)
	}
	return nil
}

// ComputeGridAggregation groups locations of the given layerType into lat/lng
// grid cells of size gridSizeKm (approximated as degrees: 1 deg ≈ 111 km),
// counts the locations per cell, and saves the result to spatial_aggregates.
func (r *AnalyticsRepository) ComputeGridAggregation(layerType, metric string, gridSizeKm float64) error {
	locs, err := r.GetLocations(layerType)
	if err != nil {
		return fmt.Errorf("analytics: ComputeGridAggregation: fetch locations: %w", err)
	}

	const degPerKm = 1.0 / 111.0
	gridStep := gridSizeKm * degPerKm

	counts := make(map[string]float64)
	for _, loc := range locs {
		if loc.Lat == nil || loc.Lng == nil {
			continue
		}
		cellLat := math.Floor(*loc.Lat/gridStep) * gridStep
		cellLng := math.Floor(*loc.Lng/gridStep) * gridStep
		cellKey := fmt.Sprintf("%.6f,%.6f", cellLat, cellLng)
		counts[cellKey]++
	}

	for cellKey, count := range counts {
		if err := r.UpsertSpatialAggregate(layerType, cellKey, metric, count); err != nil {
			return fmt.Errorf("analytics: ComputeGridAggregation: upsert %q: %w", cellKey, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------
// Extended spatial analysis
// ---------------------------------------------------------------

// haversineKm returns the great-circle distance in kilometres between two
// lat/lng points using the Haversine formula.
func haversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusKm = 6371.0
	dLat := (lat2 - lat1) * math.Pi / 180.0
	dLng := (lng2 - lng1) * math.Pi / 180.0
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180.0)*math.Cos(lat2*math.Pi/180.0)*
			math.Sin(dLng/2)*math.Sin(dLng/2)
	return earthRadiusKm * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// LocationsWithinRadius returns all locations (optionally filtered by locType)
// whose geodesic distance from (centerLat, centerLng) is ≤ radiusKm.
// Uses the Haversine formula computed in Go (no spatial extension required).
func (r *AnalyticsRepository) LocationsWithinRadius(centerLat, centerLng, radiusKm float64, locType string) ([]models.Location, error) {
	all, err := r.GetLocations(locType)
	if err != nil {
		return nil, fmt.Errorf("analytics: LocationsWithinRadius: %w", err)
	}

	var out []models.Location
	for _, loc := range all {
		if loc.Lat == nil || loc.Lng == nil {
			continue
		}
		if haversineKm(centerLat, centerLng, *loc.Lat, *loc.Lng) <= radiusKm {
			out = append(out, loc)
		}
	}
	return out, nil
}

// POIDensityWithinRadius counts the number of locations of each type within
// radiusKm of (centerLat, centerLng).  Returns a map of locationType → count.
func (r *AnalyticsRepository) POIDensityWithinRadius(centerLat, centerLng, radiusKm float64) (map[string]int, error) {
	locs, err := r.LocationsWithinRadius(centerLat, centerLng, radiusKm, "")
	if err != nil {
		return nil, fmt.Errorf("analytics: POIDensityWithinRadius: %w", err)
	}

	density := make(map[string]int)
	for _, loc := range locs {
		t := "unknown"
		if loc.Type != nil {
			t = *loc.Type
		}
		density[t]++
	}
	return density, nil
}

// TrajectoryPoints returns the ordered sequence of distribution scan events
// for a given material, forming its custody chain / trajectory.
func (r *AnalyticsRepository) TrajectoryPoints(materialID int64) ([]models.TrajectoryPoint, error) {
	const q = `
		SELECT
			COALESCE(scan_id, ''),
			event_type,
			COALESCE(custody_from, ''),
			COALESCE(custody_to,   ''),
			occurred_at
		FROM   distribution_events
		WHERE  material_id = ?
		ORDER  BY occurred_at ASC`

	rows, err := r.db.Query(q, materialID)
	if err != nil {
		return nil, fmt.Errorf("analytics: TrajectoryPoints: %w", err)
	}
	defer rows.Close()

	var out []models.TrajectoryPoint
	for rows.Next() {
		var tp models.TrajectoryPoint
		if err := rows.Scan(&tp.ScanID, &tp.EventType, &tp.CustodyFrom, &tp.CustodyTo, &tp.OccurredAt); err != nil {
			return nil, fmt.Errorf("analytics: TrajectoryPoints scan: %w", err)
		}
		out = append(out, tp)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------
// Point-in-Polygon (PiP) helpers for boundary-file containment
// ---------------------------------------------------------------

// parseWKTPolygon extracts the exterior ring from a WKT POLYGON string.
// WKT coordinate order is longitude-first: "POLYGON((lng lat, lng lat, …))".
// Returns a slice of [lat, lng] pairs, or nil if the string is not a valid polygon.
func parseWKTPolygon(wkt string) [][2]float64 {
	upper := strings.ToUpper(strings.TrimSpace(wkt))
	open := strings.Index(upper, "((")
	if open < 0 || !strings.HasPrefix(upper, "POLYGON") {
		return nil
	}
	inner := wkt[open+2:]
	end := strings.Index(inner, "))")
	if end < 0 {
		end = strings.Index(inner, ")")
		if end < 0 {
			return nil
		}
	}
	inner = inner[:end]

	parts := strings.Split(inner, ",")
	ring := make([][2]float64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		var lng, lat float64
		if n, _ := fmt.Sscanf(p, "%f %f", &lng, &lat); n == 2 {
			ring = append(ring, [2]float64{lat, lng}) // store as [lat, lng]
		}
	}
	if len(ring) < 3 {
		return nil
	}
	return ring
}

// pointInPolygon tests whether the point (lat, lng) lies inside the polygon
// ring using the ray-casting algorithm.  Ring vertices are [lat, lng] pairs.
func pointInPolygon(lat, lng float64, ring [][2]float64) bool {
	inside := false
	n := len(ring)
	j := n - 1
	for i := 0; i < n; i++ {
		iLat, iLng := ring[i][0], ring[i][1]
		jLat, jLng := ring[j][0], ring[j][1]
		if (iLat > lat) != (jLat > lat) &&
			lng < (jLng-iLng)*(lat-iLat)/(jLat-iLat)+iLng {
			inside = !inside
		}
		j = i
	}
	return inside
}

// regionEntry bundles a location with its parsed WKT polygon ring (when available).
type regionEntry struct {
	loc  models.Location
	ring [][2]float64 // non-nil when geom_wkt contains a valid POLYGON
}

// assignRegion returns the admin-region that contains (lat, lng):
//  1. Point-in-Polygon test on regions with a WKT boundary (first match wins).
//  2. Nearest-centroid fallback for regions without a WKT boundary (or when
//     no polygon contains the point).
//
// This eliminates the fixed-radius heuristic: every point is deterministically
// assigned to exactly one region.
func assignRegion(entries []regionEntry, lat, lng float64) *models.Location {
	// Pass 1: deterministic polygon containment.
	for i := range entries {
		if entries[i].ring != nil && pointInPolygon(lat, lng, entries[i].ring) {
			return &entries[i].loc
		}
	}
	// Pass 2: nearest centroid for regions without boundary files.
	var best *models.Location
	bestDist := math.MaxFloat64
	for i := range entries {
		loc := &entries[i].loc
		if loc.Lat == nil || loc.Lng == nil {
			continue
		}
		if d := haversineKm(lat, lng, *loc.Lat, *loc.Lng); d < bestDist {
			bestDist = d
			best = loc
		}
	}
	return best
}

// regionEventPoint is an internal type used during region-stats computation.
type regionEventPoint struct {
	id          int64
	orderID     int64
	custodyFrom string
	custodyTo   string
}

// RegionStats aggregates order and distribution-scan counts per admin-region.
//
// Each non-region location referenced by a distribution event's custody_to or
// custody_from field is assigned to an administrative region using the
// assignRegion helper:
//   - Point-in-Polygon containment when the region carries a WKT polygon boundary.
//   - Nearest-centroid as a deterministic fallback when no boundary file exists.
//
// This replaces the former fixed-radius (50 km) heuristic.
func (r *AnalyticsRepository) RegionStats() ([]models.RegionStat, error) {
	// 1. Load all admin-region locations and parse their WKT polygons.
	regions, err := r.GetLocations("admin_region")
	if err != nil {
		return nil, fmt.Errorf("analytics: RegionStats: fetch regions: %w", err)
	}

	entries := make([]regionEntry, len(regions))
	for i, reg := range regions {
		e := regionEntry{loc: reg}
		if reg.GeomWKT != nil {
			e.ring = parseWKTPolygon(*reg.GeomWKT)
		}
		entries[i] = e
	}

	// 2. Build name → (lat, lng) map for all non-region locations.
	type locPt struct{ lat, lng float64 }
	const locQ = `
		SELECT name, lat, lng
		FROM   locations
		WHERE  (type IS NULL OR type != 'admin_region')
		  AND  lat IS NOT NULL AND lng IS NOT NULL`
	locRows, err := r.db.Query(locQ)
	if err != nil {
		return nil, fmt.Errorf("analytics: RegionStats: fetch locations: %w", err)
	}
	defer locRows.Close()
	locByName := make(map[string]locPt)
	for locRows.Next() {
		var name string
		var pt locPt
		if err := locRows.Scan(&name, &pt.lat, &pt.lng); err != nil {
			continue
		}
		locByName[name] = pt
	}
	locRows.Close()

	// 3. Load all distribution events.
	const deQ = `
		SELECT id, COALESCE(order_id, 0),
		       COALESCE(custody_from, ''), COALESCE(custody_to, '')
		FROM   distribution_events`
	deRows, err := r.db.Query(deQ)
	if err != nil {
		return nil, fmt.Errorf("analytics: RegionStats: fetch events: %w", err)
	}
	defer deRows.Close()
	var events []regionEventPoint
	for deRows.Next() {
		var ev regionEventPoint
		if err := deRows.Scan(&ev.id, &ev.orderID, &ev.custodyFrom, &ev.custodyTo); err != nil {
			continue
		}
		events = append(events, ev)
	}
	deRows.Close()

	// 4. Assign each custody location to a region; accumulate per-region counts.
	//    location name → region ID
	locRegion := make(map[string]int64)
	for name, pt := range locByName {
		reg := assignRegion(entries, pt.lat, pt.lng)
		if reg != nil {
			locRegion[name] = reg.ID
		}
	}

	type regionCount struct{ orders, scans int }
	regionAgg := make(map[int64]struct {
		orderSet map[int64]bool
		scans    int
	})
	for _, ev := range events {
		for _, name := range []string{ev.custodyFrom, ev.custodyTo} {
			regID, ok := locRegion[name]
			if !ok || name == "" {
				continue
			}
			rc := regionAgg[regID]
			if rc.orderSet == nil {
				rc.orderSet = make(map[int64]bool)
			}
			rc.scans++
			if ev.orderID > 0 {
				rc.orderSet[ev.orderID] = true
			}
			regionAgg[regID] = rc
		}
	}

	// 5. Build the output slice (include all regions, even zero-count ones).
	out := make([]models.RegionStat, 0, len(regions))
	for _, reg := range regions {
		if reg.Lat == nil || reg.Lng == nil {
			continue
		}
		rc := regionAgg[reg.ID]
		out = append(out, models.RegionStat{
			RegionName: reg.Name,
			OrderCount: len(rc.orderSet),
			ScanCount:  rc.scans,
			Lat:        *reg.Lat,
			Lng:        *reg.Lng,
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].RegionName < out[j].RegionName })
	return out, nil
}

// ComputeRegionAggregation recomputes region-level spatial aggregates and
// stores them in spatial_aggregates (layer_type='admin_region').  Uses the
// same Haversine-based containment logic as RegionStats so results are consistent.
func (r *AnalyticsRepository) ComputeRegionAggregation(metric string) error {
	stats, err := r.RegionStats()
	if err != nil {
		return fmt.Errorf("analytics: ComputeRegionAggregation: %w", err)
	}

	for _, rs := range stats {
		cellKey := fmt.Sprintf("%.6f,%.6f", rs.Lat, rs.Lng)
		var value float64
		switch metric {
		case "orders":
			value = float64(rs.OrderCount)
		case "scans":
			value = float64(rs.ScanCount)
		default: // "count" or unknown — sum of both
			value = float64(rs.OrderCount + rs.ScanCount)
		}
		if err := r.UpsertSpatialAggregate("admin_region", cellKey, metric, value); err != nil {
			return fmt.Errorf("analytics: ComputeRegionAggregation: upsert %q: %w", cellKey, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------
// Exports
// ---------------------------------------------------------------

// ExportOrders returns a flat slice of order rows for CSV export.  All
// filter parameters are optional (pass "" to skip).
func (r *AnalyticsRepository) ExportOrders(status, dateFrom, dateTo string) ([]OrderExportRow, error) {
	where := "1=1"
	args := []interface{}{}

	if status != "" {
		where += " AND o.status = ?"
		args = append(args, status)
	}
	if dateFrom != "" {
		where += " AND o.created_at >= ?"
		args = append(args, dateFrom)
	}
	if dateTo != "" {
		where += " AND o.created_at <= ?"
		args = append(args, dateTo)
	}

	q := `
		SELECT
			o.id,
			u.username,
			u.email,
			o.status,
			o.total_amount,
			o.created_at,
			COUNT(oi.id) AS item_count
		FROM   orders o
		JOIN   users u  ON u.id = o.user_id
		LEFT   JOIN order_items oi ON oi.order_id = o.id
		WHERE  ` + where + `
		GROUP  BY o.id, u.username, u.email, o.status, o.total_amount, o.created_at
		ORDER  BY o.created_at DESC`

	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("analytics: ExportOrders: %w", err)
	}
	defer rows.Close()

	var out []OrderExportRow
	for rows.Next() {
		var row OrderExportRow
		if err := rows.Scan(
			&row.OrderID, &row.UserName, &row.UserEmail,
			&row.Status, &row.TotalAmount, &row.CreatedAt, &row.ItemCount,
		); err != nil {
			return nil, fmt.Errorf("analytics: ExportOrders scan: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// ExportDistribution returns a flat slice of distribution-event rows for CSV
// export, optionally filtered by date range.
func (r *AnalyticsRepository) ExportDistribution(dateFrom, dateTo string) ([]DistribExportRow, error) {
	where := "1=1"
	args := []interface{}{}

	if dateFrom != "" {
		where += " AND de.occurred_at >= ?"
		args = append(args, dateFrom)
	}
	if dateTo != "" {
		where += " AND de.occurred_at <= ?"
		args = append(args, dateTo)
	}

	q := `
		SELECT
			COALESCE(de.scan_id, ''),
			de.event_type,
			m.title,
			COALESCE(u.username, ''),
			de.occurred_at
		FROM   distribution_events de
		JOIN   materials m ON m.id = de.material_id
		LEFT   JOIN users u ON u.id = de.actor_id
		WHERE  ` + where + `
		ORDER  BY de.occurred_at DESC`

	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("analytics: ExportDistribution: %w", err)
	}
	defer rows.Close()

	var out []DistribExportRow
	for rows.Next() {
		var row DistribExportRow
		if err := rows.Scan(
			&row.ScanID, &row.EventType, &row.MaterialTitle,
			&row.ActorName, &row.OccurredAt,
		); err != nil {
			return nil, fmt.Errorf("analytics: ExportDistribution scan: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------
// Dashboard stat counts (used by GET /api/stats/:stat)
// ---------------------------------------------------------------

// CountOrdersForUser returns the number of orders placed by a specific user.
func (r *AnalyticsRepository) CountOrdersForUser(userID int64) (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM orders WHERE user_id = ?`, userID).Scan(&n)
	return n, err
}

// CountFavoritesListsForUser returns the number of favorites lists owned by a user.
func (r *AnalyticsRepository) CountFavoritesListsForUser(userID int64) (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM favorites_lists WHERE user_id = ?`, userID).Scan(&n)
	return n, err
}

// CountRecentMaterialViewsForUser returns the number of distinct materials
// viewed by the user in the last 30 days.
func (r *AnalyticsRepository) CountRecentMaterialViewsForUser(userID int64) (int, error) {
	var n int
	err := r.db.QueryRow(`
		SELECT COUNT(DISTINCT material_id)
		FROM   browse_history
		WHERE  user_id = ?
		  AND  visited_at >= datetime('now', '-30 days')`,
		userID,
	).Scan(&n)
	return n, err
}

// TotalOrderCount returns the total number of orders across all users.
func (r *AnalyticsRepository) TotalOrderCount() (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM orders`).Scan(&n)
	return n, err
}

// PendingReturnRequestCount returns the number of return requests in 'pending' status.
func (r *AnalyticsRepository) PendingReturnRequestCount() (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM return_requests WHERE status = 'pending'`).Scan(&n)
	return n, err
}

// PendingIssueCount returns the number of order items awaiting distribution.
func (r *AnalyticsRepository) PendingIssueCount() (int, error) {
	var n int
	err := r.db.QueryRow(`
		SELECT COUNT(*)
		FROM   order_items oi
		JOIN   orders o ON o.id = oi.order_id
		WHERE  o.status = 'pending_shipment'
		  AND  oi.fulfillment_status = 'pending'`).Scan(&n)
	return n, err
}

// BackorderCount returns the number of unresolved backorder records.
func (r *AnalyticsRepository) BackorderCount() (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM backorders WHERE resolved_at IS NULL`).Scan(&n)
	return n, err
}

// InstructorPendingApprovalCount returns the count of return requests pending
// approval. Instructors approve/reject return requests.
func (r *AnalyticsRepository) InstructorPendingApprovalCount() (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM return_requests WHERE status = 'pending'`).Scan(&n)
	return n, err
}

// ModerationQueueCount returns the number of comments currently in the moderation queue.
func (r *AnalyticsRepository) ModerationQueueCount() (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM comments WHERE status = 'collapsed'`).Scan(&n)
	return n, err
}

// ---------------------------------------------------------------
// Extended KPI metrics
// ---------------------------------------------------------------

// GMV returns Gross Merchandise Value: the sum of total_amount for completed
// orders in the given period ("7d", "30d", "90d").
func (r *AnalyticsRepository) GMV(period string) (float64, error) {
	mod := periodExpr(period)
	const q = `
		SELECT COALESCE(SUM(total_amount), 0)
		FROM   orders
		WHERE  status = 'completed'
		  AND  created_at >= datetime('now', ?)`

	var v float64
	if err := r.db.QueryRow(q, mod).Scan(&v); err != nil {
		return 0, fmt.Errorf("analytics: GMV: %w", err)
	}
	return v, nil
}

// AOV returns Average Order Value: GMV / number of completed orders in the
// given period.  Returns 0 when there are no completed orders.
func (r *AnalyticsRepository) AOV(period string) (float64, error) {
	mod := periodExpr(period)
	const q = `
		SELECT
			COALESCE(SUM(total_amount), 0) /
			NULLIF(CAST(COUNT(*) AS REAL), 0)
		FROM   orders
		WHERE  status = 'completed'
		  AND  created_at >= datetime('now', ?)`

	var v sql.NullFloat64
	if err := r.db.QueryRow(q, mod).Scan(&v); err != nil {
		return 0, fmt.Errorf("analytics: AOV: %w", err)
	}
	if !v.Valid {
		return 0, nil
	}
	return v.Float64, nil
}

// ConversionRate returns the fraction of registered users (non-deleted) who
// have placed at least one order, expressed as a value 0–100.
func (r *AnalyticsRepository) ConversionRate() (float64, error) {
	const q = `
		SELECT
			CAST(COUNT(DISTINCT o.user_id) AS REAL) /
			NULLIF(COUNT(DISTINCT u.id), 0) * 100
		FROM   users u
		LEFT   JOIN orders o ON o.user_id = u.id
		WHERE  u.deleted_at IS NULL`

	var rate sql.NullFloat64
	if err := r.db.QueryRow(q).Scan(&rate); err != nil {
		return 0, fmt.Errorf("analytics: ConversionRate: %w", err)
	}
	if !rate.Valid {
		return 0, nil
	}
	return rate.Float64, nil
}

// RepeatPurchaseRate returns the fraction of ordering users who have placed
// 2 or more orders, expressed as a value 0–100.
func (r *AnalyticsRepository) RepeatPurchaseRate() (float64, error) {
	const q = `
		WITH user_orders AS (
			SELECT user_id, COUNT(*) AS order_count
			FROM   orders
			GROUP  BY user_id
		)
		SELECT
			CAST(SUM(CASE WHEN order_count >= 2 THEN 1 ELSE 0 END) AS REAL) /
			NULLIF(COUNT(*), 0) * 100
		FROM user_orders`

	var rate sql.NullFloat64
	if err := r.db.QueryRow(q).Scan(&rate); err != nil {
		return 0, fmt.Errorf("analytics: RepeatPurchaseRate: %w", err)
	}
	if !rate.Valid {
		return 0, nil
	}
	return rate.Float64, nil
}

// FunnelStage represents the order count at a given status stage in the funnel.
type FunnelStage struct {
	Stage string
	Count int
}

// OrderFunnel returns the drop-off counts at each order pipeline stage in the
// canonical order: pending_payment → pending_shipment → in_transit → completed.
// The canceled stage is also included for reference.
func (r *AnalyticsRepository) OrderFunnel() ([]FunnelStage, error) {
	const q = `
		SELECT status, COUNT(*) AS cnt
		FROM   orders
		WHERE  status IN ('pending_payment', 'pending_shipment', 'in_transit', 'completed', 'canceled')
		GROUP  BY status`

	rows, err := r.db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("analytics: OrderFunnel: %w", err)
	}
	defer rows.Close()

	// Build a map first, then return in canonical stage order.
	counts := make(map[string]int)
	for rows.Next() {
		var status string
		var cnt int
		if err := rows.Scan(&status, &cnt); err != nil {
			return nil, fmt.Errorf("analytics: OrderFunnel scan: %w", err)
		}
		counts[status] = cnt
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	order := []string{"pending_payment", "pending_shipment", "in_transit", "completed", "canceled"}
	out := make([]FunnelStage, 0, len(order))
	for _, s := range order {
		out = append(out, FunnelStage{Stage: s, Count: counts[s]})
	}
	return out, nil
}
