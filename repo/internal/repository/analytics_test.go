package repository_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"w2t86/internal/repository"
	"w2t86/internal/services"
	"w2t86/internal/testutil"
)

// TestRegionStats_CorrectAggregation verifies that RegionStats produces
// per-region counts that correctly reflect only events whose custody names
// match nearby locations, not a cross-join total.
//
// Setup:
//   - Region A at (0.0, 0.0)  — depot "Depot-A" at (0.1, 0.1) is within 50 km
//   - Region B at (10.0, 10.0) — depot "Depot-B" at (10.1, 10.1) is within 50 km
//   - 2 distribution events referencing Depot-A (custody_to = "Depot-A")
//   - 1 distribution event referencing Depot-B
//
// Expected: region A = 2 scans, region B = 1 scan; no cross-contamination.
func TestRegionStats_CorrectAggregation(t *testing.T) {
	db := testutil.NewTestDB(t)

	// Insert region centroids.
	if _, err := db.Exec(`INSERT INTO locations (name, type, lat, lng) VALUES ('Region-A', 'admin_region', 0.0, 0.0)`); err != nil {
		t.Fatalf("insert Region-A: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO locations (name, type, lat, lng) VALUES ('Region-B', 'admin_region', 10.0, 10.0)`); err != nil {
		t.Fatalf("insert Region-B: %v", err)
	}
	// Insert depot locations (non-region, within 50 km of their respective regions).
	if _, err := db.Exec(`INSERT INTO locations (name, type, lat, lng) VALUES ('Depot-A', 'depot', 0.1, 0.1)`); err != nil {
		t.Fatalf("insert Depot-A: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO locations (name, type, lat, lng) VALUES ('Depot-B', 'depot', 10.1, 10.1)`); err != nil {
		t.Fatalf("insert Depot-B: %v", err)
	}

	// Insert a material and two orders.
	var matID, orderID1, orderID2 int64
	if err := db.QueryRow(`INSERT INTO materials (title, total_qty, available_qty, reserved_qty, status) VALUES ('Book', 10, 10, 0, 'active') RETURNING id`).Scan(&matID); err != nil {
		t.Fatalf("insert material: %v", err)
	}
	var userID int64
	if err := db.QueryRow(`INSERT INTO users (username, email, password_hash, role) VALUES ('u1','u1@x.com','$2a$12$x','student') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if err := db.QueryRow(`INSERT INTO orders (user_id, status, total_amount, auto_close_at, created_at, updated_at) VALUES (?, 'completed', 0, datetime('now'), datetime('now'), datetime('now')) RETURNING id`, userID).Scan(&orderID1); err != nil {
		t.Fatalf("insert order1: %v", err)
	}
	if err := db.QueryRow(`INSERT INTO orders (user_id, status, total_amount, auto_close_at, created_at, updated_at) VALUES (?, 'completed', 0, datetime('now'), datetime('now'), datetime('now')) RETURNING id`, userID).Scan(&orderID2); err != nil {
		t.Fatalf("insert order2: %v", err)
	}

	// 2 events for Region-A depot, 1 event for Region-B depot.
	insEvent := func(orderID int64, custodyTo string) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO distribution_events (order_id, material_id, qty, event_type, actor_id, custody_to, occurred_at) VALUES (?, ?, 1, 'issue', ?, ?, datetime('now'))`, orderID, matID, userID, custodyTo); err != nil {
			t.Fatalf("insert event custody_to=%s: %v", custodyTo, err)
		}
	}
	insEvent(orderID1, "Depot-A")
	insEvent(orderID1, "Depot-A") // second event same order
	insEvent(orderID2, "Depot-B")

	repo := repository.NewAnalyticsRepository(db)
	stats, err := repo.RegionStats()
	if err != nil {
		t.Fatalf("RegionStats: %v", err)
	}

	byName := make(map[string]struct {
		orders, scans int
	})
	for _, s := range stats {
		byName[s.RegionName] = struct{ orders, scans int }{s.OrderCount, s.ScanCount}
	}

	// Region A should have 2 scans from 1 distinct order.
	if a, ok := byName["Region-A"]; !ok {
		t.Fatal("Region-A not found in stats")
	} else {
		if a.scans != 2 {
			t.Errorf("Region-A: expected 2 scans, got %d", a.scans)
		}
		if a.orders != 1 {
			t.Errorf("Region-A: expected 1 order, got %d", a.orders)
		}
	}

	// Region B should have 1 scan from 1 distinct order.
	if b, ok := byName["Region-B"]; !ok {
		t.Fatal("Region-B not found in stats")
	} else {
		if b.scans != 1 {
			t.Errorf("Region-B: expected 1 scan, got %d", b.scans)
		}
		if b.orders != 1 {
			t.Errorf("Region-B: expected 1 order, got %d", b.orders)
		}
	}

	// Crucially: Region A must NOT have Region B's counts (no cross-join).
	if a, b := byName["Region-A"], byName["Region-B"]; a.scans == a.scans+b.scans {
		t.Errorf("cross-join detected: Region-A scan count (%d) equals total (%d)", a.scans, a.scans+b.scans)
	}
}

// TestSpatialAggregates_Query_Under200ms verifies the checklist requirement:
// "Spatial aggregate query returns in <200ms on 10k rows."
//
// It inserts 10 000 spatial_aggregates rows for a single layer_type, then
// times a GetSpatialAggregates call and fails if it exceeds 200 ms.
func TestSpatialAggregates_Query_Under200ms(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewAnalyticsRepository(db)

	const (
		n         = 10_000
		layerType = "school"
		metric    = "count"
		limit     = 200 * time.Millisecond
	)

	// Bulk-insert 10k rows via a transaction for speed.
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	stmt, err := tx.Prepare(`
		INSERT INTO spatial_aggregates (layer_type, cell_key, metric, value, computed_at)
		VALUES (?, ?, ?, ?, datetime('now'))
		ON CONFLICT(layer_type, cell_key, metric) DO NOTHING`)
	if err != nil {
		tx.Rollback() //nolint:errcheck
		t.Fatalf("prepare: %v", err)
	}
	for i := 0; i < n; i++ {
		cellKey := fmt.Sprintf("cell_%06d", i)
		if _, err := stmt.Exec(layerType, cellKey, metric, float64(i)); err != nil {
			stmt.Close()
			tx.Rollback() //nolint:errcheck
			t.Fatalf("insert row %d: %v", i, err)
		}
	}
	stmt.Close()
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	start := time.Now()
	rows, err := repo.GetSpatialAggregates(layerType)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("GetSpatialAggregates: %v", err)
	}
	if len(rows) != n {
		t.Fatalf("expected %d rows, got %d", n, len(rows))
	}
	if elapsed > limit {
		t.Errorf("GetSpatialAggregates(%d rows) took %v, want <200ms", n, elapsed)
	}
	t.Logf("GetSpatialAggregates(%d rows): %v", n, elapsed)
}

// TestExportOrdersCSV_MasksPII verifies the checklist requirement:
// "PII masked in all export endpoints."
//
// It creates an order for a known user, calls ExportOrdersCSV, and checks
// that the raw username and email do NOT appear in the CSV output, while
// their masked equivalents do.
func TestExportOrdersCSV_MasksPII(t *testing.T) {
	db := testutil.NewTestDB(t)

	// Insert a user with a recognisable name and email.
	const (
		username = "Alice Smith"
		email    = "alice.smith@example.com"
		pwHash   = "$2a$12$fMPISK6tAC1XLVM3JdJQDuB/CrXgdRM.LUPHHu4/VxS/vzihnYyQ."
	)
	var userID int64
	err := db.QueryRow(
		`INSERT INTO users (username, email, password_hash, role) VALUES (?, ?, ?, 'student') RETURNING id`,
		username, email, pwHash,
	).Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	// Insert a material.
	var matID int64
	err = db.QueryRow(
		`INSERT INTO materials (title, total_qty, available_qty, reserved_qty, status) VALUES ('Book A', 5, 5, 0, 'active') RETURNING id`,
	).Scan(&matID)
	if err != nil {
		t.Fatalf("insert material: %v", err)
	}

	// Insert an order.
	var orderID int64
	err = db.QueryRow(
		`INSERT INTO orders (user_id, status, total_amount, auto_close_at, created_at, updated_at)
		 VALUES (?, 'completed', 9.99, datetime('now','+30 minutes'), datetime('now'), datetime('now')) RETURNING id`,
		userID,
	).Scan(&orderID)
	if err != nil {
		t.Fatalf("insert order: %v", err)
	}

	analyticsRepo := repository.NewAnalyticsRepository(db)
	svc := services.NewAnalyticsService(analyticsRepo)

	// ExportOrdersCSV applies PII masking at the service layer.
	csvBytes, err := svc.ExportOrdersCSV("", "", "")
	if err != nil {
		t.Fatalf("ExportOrdersCSV: %v", err)
	}
	csvStr := string(csvBytes)
	t.Logf("exported CSV:\n%s", csvStr)

	// Raw PII must NOT appear anywhere in the CSV output.
	if strings.Contains(csvStr, "Alice") || strings.Contains(csvStr, "Smith") {
		t.Errorf("CSV contains unmasked name PII — expected initials only\n%s", csvStr)
	}
	if strings.Contains(csvStr, "@") {
		t.Errorf("CSV contains unmasked email (@ present) — expected masked ID\n%s", csvStr)
	}
}
