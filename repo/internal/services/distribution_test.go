package services_test

import (
	"database/sql"
	"testing"

	"w2t86/internal/repository"
	"w2t86/internal/services"
	"w2t86/internal/testutil"
)

// ---------------------------------------------------------------
// Fixtures / helpers
// ---------------------------------------------------------------

func newDistributionService(t *testing.T) (*services.DistributionService, *sql.DB) {
	t.Helper()
	db := testutil.NewTestDB(t)
	distRepo := repository.NewDistributionRepository(db)
	orderRepo := repository.NewOrderRepository(db)
	matRepo := repository.NewMaterialRepository(db)
	svc := services.NewDistributionService(distRepo, orderRepo, matRepo)
	return svc, db
}

// distSvcFixtures creates a user, material, and order in pending_shipment.
func distSvcFixtures(t *testing.T, db *sql.DB) (userID, matID, orderID int64) {
	t.Helper()

	r, err := db.Exec(`INSERT INTO users (username, email, password_hash, role) VALUES ('dsuser','ds@x.com','hash','clerk')`)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	userID, _ = r.LastInsertId()

	r2, err := db.Exec(`INSERT INTO materials (title, total_qty, available_qty, reserved_qty, status) VALUES ('DS Book', 10, 8, 2, 'active')`)
	if err != nil {
		t.Fatalf("insert material: %v", err)
	}
	matID, _ = r2.LastInsertId()

	r3, err := db.Exec(`INSERT INTO orders (user_id, status, total_amount) VALUES (?,'pending_shipment',10.00)`, userID)
	if err != nil {
		t.Fatalf("insert order: %v", err)
	}
	orderID, _ = r3.LastInsertId()

	if _, err := db.Exec(`INSERT INTO order_items (order_id, material_id, qty, unit_price, fulfillment_status) VALUES (?,?,2,5.00,'pending')`,
		orderID, matID); err != nil {
		t.Fatalf("insert order_item: %v", err)
	}
	return
}

// ---------------------------------------------------------------
// Tests
// ---------------------------------------------------------------

func TestDistributionService_IssueItems_Success(t *testing.T) {
	svc, db := newDistributionService(t)
	userID, matID, orderID := distSvcFixtures(t, db)

	items := []services.IssueItem{{MaterialID: matID, Qty: 2}}
	if err := svc.IssueItems(orderID, userID, "SCAN100", items); err != nil {
		t.Fatalf("IssueItems: %v", err)
	}

	// Order should advance to in_transit
	var status string
	if err := db.QueryRow(`SELECT status FROM orders WHERE id = ?`, orderID).Scan(&status); err != nil {
		t.Fatalf("query status: %v", err)
	}
	if status != "in_transit" {
		t.Errorf("expected in_transit, got %q", status)
	}
}

func TestDistributionService_RecordReturn_ReleasesInventory(t *testing.T) {
	svc, db := newDistributionService(t)
	userID, matID, orderID := distSvcFixtures(t, db)

	// Create an approved return request.
	var rrID int64
	if err := db.QueryRow(
		`INSERT INTO return_requests (order_id, user_id, type, status, requested_at)
		 VALUES (?, ?, 'return', 'approved', datetime('now')) RETURNING id`,
		orderID, userID,
	).Scan(&rrID); err != nil {
		t.Fatalf("insert return_request: %v", err)
	}

	var beforeAvail int
	if err := db.QueryRow(`SELECT available_qty FROM materials WHERE id = ?`, matID).Scan(&beforeAvail); err != nil {
		t.Fatalf("query before: %v", err)
	}

	if err := svc.RecordReturn(orderID, matID, userID, rrID, "SCAN200", 1); err != nil {
		t.Fatalf("RecordReturn: %v", err)
	}

	var afterAvail int
	if err := db.QueryRow(`SELECT available_qty FROM materials WHERE id = ?`, matID).Scan(&afterAvail); err != nil {
		t.Fatalf("query after: %v", err)
	}
	if afterAvail != beforeAvail+1 {
		t.Errorf("expected available_qty=%d, got %d", beforeAvail+1, afterAvail)
	}
}

func TestDistributionService_RecordExchange_Success(t *testing.T) {
	svc, db := newDistributionService(t)
	userID, oldMatID, orderID := distSvcFixtures(t, db)

	// Insert a new material to exchange into.
	r, err := db.Exec(`INSERT INTO materials (title, total_qty, available_qty, reserved_qty, status) VALUES ('New Book', 5, 5, 0, 'active')`)
	if err != nil {
		t.Fatalf("insert new material: %v", err)
	}
	newMatID, _ := r.LastInsertId()

	// Create an approved exchange request.
	var rrID int64
	if err := db.QueryRow(
		`INSERT INTO return_requests (order_id, user_id, type, status, requested_at)
		 VALUES (?, ?, 'exchange', 'approved', datetime('now')) RETURNING id`,
		orderID, userID,
	).Scan(&rrID); err != nil {
		t.Fatalf("insert return_request: %v", err)
	}

	if err := svc.RecordExchange(orderID, oldMatID, newMatID, userID, rrID, "SCAN300", 1); err != nil {
		t.Fatalf("RecordExchange: %v", err)
	}

	// New material available_qty should decrease by 1.
	var newAvail int
	if err := db.QueryRow(`SELECT available_qty FROM materials WHERE id = ?`, newMatID).Scan(&newAvail); err != nil {
		t.Fatalf("query new material: %v", err)
	}
	if newAvail != 4 {
		t.Errorf("expected new material available_qty=4, got %d", newAvail)
	}
}

func TestDistributionService_ReissueItem_Success(t *testing.T) {
	svc, db := newDistributionService(t)
	userID, matID, orderID := distSvcFixtures(t, db)

	var beforeAvail int
	if err := db.QueryRow(`SELECT available_qty FROM materials WHERE id = ?`, matID).Scan(&beforeAvail); err != nil {
		t.Fatalf("query before available_qty: %v", err)
	}

	if err := svc.ReissueItem(orderID, matID, userID, "OLD_SCAN", "NEW_SCAN", "lost"); err != nil {
		t.Fatalf("ReissueItem (lost): %v", err)
	}

	// Two events must be recorded: one "lost" event and one "issued" event.
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM distribution_events WHERE order_id = ?`, orderID).Scan(&count); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if count < 2 {
		t.Errorf("expected at least 2 distribution events, got %d", count)
	}

	// Inventory must be decremented by 1 for the replacement copy.
	var afterAvail int
	if err := db.QueryRow(`SELECT available_qty FROM materials WHERE id = ?`, matID).Scan(&afterAvail); err != nil {
		t.Fatalf("query after available_qty: %v", err)
	}
	if afterAvail != beforeAvail-1 {
		t.Errorf("expected available_qty=%d after reissue, got %d (inventory not decremented)", beforeAvail-1, afterAvail)
	}
}

func TestDistributionService_ReissueItem_ZeroStock_Fails(t *testing.T) {
	svc, db := newDistributionService(t)
	userID, matID, orderID := distSvcFixtures(t, db)

	// Drive available_qty to zero.
	if _, err := db.Exec(`UPDATE materials SET available_qty = 0 WHERE id = ?`, matID); err != nil {
		t.Fatalf("set available_qty=0: %v", err)
	}

	err := svc.ReissueItem(orderID, matID, userID, "OLD2", "NEW2", "damaged")
	if err == nil {
		t.Error("expected error when available_qty=0, got nil")
	}
}

func TestDistributionService_ReissueItem_InvalidReason_Fails(t *testing.T) {
	svc, db := newDistributionService(t)
	userID, matID, orderID := distSvcFixtures(t, db)

	err := svc.ReissueItem(orderID, matID, userID, "OLD_SCAN", "NEW_SCAN", "stolen")
	if err == nil {
		t.Error("expected error for invalid reason, got nil")
	}
}

// TestIssueItems_PartialFulfillment_CreatesBackorder verifies that when
// IssuedQty < Qty the order_item is marked "backordered" and a backorder record
// is written with qty equal to the shortfall.
func TestIssueItems_PartialFulfillment_CreatesBackorder(t *testing.T) {
	svc, db := newDistributionService(t)
	userID, matID, orderID := distSvcFixtures(t, db)

	// Issue only 1 of the 2 ordered copies.
	items := []services.IssueItem{{MaterialID: matID, Qty: 2, IssuedQty: 1}}
	if err := svc.IssueItems(orderID, userID, "SCAN_PARTIAL", items); err != nil {
		t.Fatalf("IssueItems (partial): %v", err)
	}

	// fulfillment_status must be "backordered".
	var fulfillStatus string
	if err := db.QueryRow(
		`SELECT fulfillment_status FROM order_items WHERE order_id = ? AND material_id = ?`,
		orderID, matID,
	).Scan(&fulfillStatus); err != nil {
		t.Fatalf("query fulfillment_status: %v", err)
	}
	if fulfillStatus != "backordered" {
		t.Errorf("expected fulfillment_status 'backordered', got %q", fulfillStatus)
	}

	// A backorder row should exist with qty == shortfall (2 - 1 = 1).
	var backorderQty int
	if err := db.QueryRow(`
		SELECT b.qty
		FROM   backorders b
		JOIN   order_items oi ON oi.id = b.order_item_id
		WHERE  oi.order_id = ? AND oi.material_id = ?`,
		orderID, matID,
	).Scan(&backorderQty); err != nil {
		t.Fatalf("query backorder: %v", err)
	}
	if backorderQty != 1 {
		t.Errorf("expected backorder qty=1 (shortfall), got %d", backorderQty)
	}
}

// TestIssueItems_FullFulfillment_MarksItemFulfilled verifies that issuing the
// exact ordered quantity sets fulfillment_status to "fulfilled" with no backorder.
func TestIssueItems_FullFulfillment_MarksItemFulfilled(t *testing.T) {
	svc, db := newDistributionService(t)
	userID, matID, orderID := distSvcFixtures(t, db)

	// Issue the full ordered quantity.
	items := []services.IssueItem{{MaterialID: matID, Qty: 2, IssuedQty: 2}}
	if err := svc.IssueItems(orderID, userID, "SCAN_FULL", items); err != nil {
		t.Fatalf("IssueItems (full): %v", err)
	}

	// fulfillment_status must be "fulfilled".
	var fulfillStatus string
	if err := db.QueryRow(
		`SELECT fulfillment_status FROM order_items WHERE order_id = ? AND material_id = ?`,
		orderID, matID,
	).Scan(&fulfillStatus); err != nil {
		t.Fatalf("query fulfillment_status: %v", err)
	}
	if fulfillStatus != "fulfilled" {
		t.Errorf("expected fulfillment_status 'fulfilled', got %q", fulfillStatus)
	}

	// No backorder should exist for this order.
	var backorderCount int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM   backorders b
		JOIN   order_items oi ON oi.id = b.order_item_id
		WHERE  oi.order_id = ?`, orderID,
	).Scan(&backorderCount); err != nil {
		t.Fatalf("query backorder count: %v", err)
	}
	if backorderCount != 0 {
		t.Errorf("expected 0 backorders for full fulfillment, got %d", backorderCount)
	}
}

// TestDistributionService_RecordReturn_MaterialNotInOrder verifies that
// RecordReturn rejects a materialID that is not part of the order's line items.
func TestDistributionService_RecordReturn_MaterialNotInOrder(t *testing.T) {
	svc, db := newDistributionService(t)
	userID, _, orderID := distSvcFixtures(t, db)

	// Insert a second material that is NOT part of the order.
	r, err := db.Exec(`INSERT INTO materials (title, total_qty, available_qty, reserved_qty, status) VALUES ('Unrelated Book', 5, 5, 0, 'active')`)
	if err != nil {
		t.Fatalf("insert unrelated material: %v", err)
	}
	unrelatedMatID, _ := r.LastInsertId()

	// Create an approved return request.
	var rrID int64
	if err := db.QueryRow(
		`INSERT INTO return_requests (order_id, user_id, type, status, requested_at)
		 VALUES (?, ?, 'return', 'approved', datetime('now')) RETURNING id`,
		orderID, userID,
	).Scan(&rrID); err != nil {
		t.Fatalf("insert return_request: %v", err)
	}

	// Attempt to return the unrelated material — must fail.
	err = svc.RecordReturn(orderID, unrelatedMatID, userID, rrID, "SCAN_UNRELATED", 1)
	if err == nil {
		t.Error("expected error when materialID is not part of order, got nil")
	}
}

// TestDistributionService_RecordReturn_OverReturn verifies that RecordReturn
// rejects a return quantity that exceeds the originally ordered quantity.
func TestDistributionService_RecordReturn_OverReturn(t *testing.T) {
	svc, db := newDistributionService(t)
	userID, matID, orderID := distSvcFixtures(t, db)

	// Create an approved return request.
	var rrID int64
	if err := db.QueryRow(
		`INSERT INTO return_requests (order_id, user_id, type, status, requested_at)
		 VALUES (?, ?, 'return', 'approved', datetime('now')) RETURNING id`,
		orderID, userID,
	).Scan(&rrID); err != nil {
		t.Fatalf("insert return_request: %v", err)
	}

	// The fixture inserts qty=2 in order_items; returning 999 must fail.
	err := svc.RecordReturn(orderID, matID, userID, rrID, "SCAN_OVER", 999)
	if err == nil {
		t.Error("expected error when return qty exceeds ordered qty, got nil")
	}
}

// TestDistributionService_RecordExchange_OldMaterialNotInOrder verifies that
// RecordExchange rejects an oldMaterialID not present in the order's line items.
func TestDistributionService_RecordExchange_OldMaterialNotInOrder(t *testing.T) {
	svc, db := newDistributionService(t)
	userID, _, orderID := distSvcFixtures(t, db)

	// Insert materials: one unrelated (old) and one valid (new).
	r, err := db.Exec(`INSERT INTO materials (title, total_qty, available_qty, reserved_qty, status) VALUES ('Unrelated Old', 5, 5, 0, 'active')`)
	if err != nil {
		t.Fatalf("insert unrelated material: %v", err)
	}
	unrelatedOldID, _ := r.LastInsertId()

	r2, err := db.Exec(`INSERT INTO materials (title, total_qty, available_qty, reserved_qty, status) VALUES ('New Book', 5, 5, 0, 'active')`)
	if err != nil {
		t.Fatalf("insert new material: %v", err)
	}
	newMatID, _ := r2.LastInsertId()

	// Create an approved exchange request.
	var rrID int64
	if err := db.QueryRow(
		`INSERT INTO return_requests (order_id, user_id, type, status, requested_at)
		 VALUES (?, ?, 'exchange', 'approved', datetime('now')) RETURNING id`,
		orderID, userID,
	).Scan(&rrID); err != nil {
		t.Fatalf("insert return_request: %v", err)
	}

	// oldMaterialID is not part of the order — must fail.
	err = svc.RecordExchange(orderID, unrelatedOldID, newMatID, userID, rrID, "SCAN_EXCH", 1)
	if err == nil {
		t.Error("expected error when oldMaterialID is not part of order, got nil")
	}
}
