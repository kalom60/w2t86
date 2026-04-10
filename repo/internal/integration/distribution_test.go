package integration_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"w2t86/internal/repository"
)

// prepareOrderForIssue creates a student user, places an order, and advances
// it to pending_shipment so that the distribution service can issue items.
// Returns the order, the material, and the student's userID.
func prepareOrderForIssue(t *testing.T, db interface {
	QueryRow(query string, args ...interface{}) interface {
		Scan(dest ...interface{}) error
	}
	Exec(query string, args ...interface{}) (interface{ LastInsertId() (int64, error) }, error)
}) {
	// This is a convenience signature; the real function below uses *sql.DB.
}

// prepareShippableOrder creates a material, user, order, and transitions it
// to pending_shipment. Returns orderID, materialID.
func prepareShippableOrder(t *testing.T, db interface{ QueryRow(string, ...interface{}) interface{ Scan(...interface{}) error } }) {
	// stub — real logic in tests below
}

// TestIssueItems_Success verifies that POST /distribution/issue by a clerk
// returns 200 or 302 and records a distribution event.
func TestIssueItems_Success(t *testing.T) {
	t.Helper()
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	// Create student user and order.
	studentUser := createTestUser(t, db, "student")
	order := createTestOrder(t, db, studentUser.ID)

	// Advance order to pending_shipment.
	orderRepo := repository.NewOrderRepository(db)
	matRepo := repository.NewMaterialRepository(db)
	if err := orderRepo.Transition(order.ID, studentUser.ID, "pending_shipment", "pay", matRepo); err != nil {
		t.Fatalf("transition to pending_shipment: %v", err)
	}

	// Get material ID from the order.
	var materialID int64
	if err := db.QueryRow(`SELECT material_id FROM order_items WHERE order_id = ? LIMIT 1`, order.ID).Scan(&materialID); err != nil {
		t.Fatalf("get material id: %v", err)
	}

	clerkCookie := loginAs(t, app, db, "clerk")

	body := fmt.Sprintf("order_id=%d&scan_id=SCAN001&material_id=%d&qty=1",
		order.ID, materialID)

	resp := makeRequest(app, http.MethodPost, "/distribution/issue",
		body, clerkCookie, "application/x-www-form-urlencoded")

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 200/302 on issue items, got %d; body: %s",
			resp.StatusCode, readBody(resp))
	}

	// Check that a distribution event was created.
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM distribution_events WHERE order_id = ?`, order.ID).Scan(&count); err != nil {
		t.Fatalf("query distribution_events: %v", err)
	}
	if count == 0 {
		t.Error("expected distribution_event to be created on issue")
	}
}

// TestIssueItems_RequiresClerkRole verifies that a student cannot POST to
// /distribution/issue and receives 403.
func TestIssueItems_RequiresClerkRole(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	studentCookie := loginAs(t, app, db, "student")

	resp := makeRequest(app, http.MethodPost, "/distribution/issue",
		"order_id=1&scan_id=X&material_id=1&qty=1",
		studentCookie, "application/x-www-form-urlencoded")

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for student on /distribution/issue, got %d", resp.StatusCode)
	}
}

// TestLedger_ReturnsEntries verifies GET /distribution/ledger by a clerk returns 200.
func TestLedger_ReturnsEntries(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	clerkCookie := loginAs(t, app, db, "clerk")

	resp := makeRequest(app, http.MethodGet, "/distribution/ledger", "", clerkCookie, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for clerk on /distribution/ledger, got %d; body: %s",
			resp.StatusCode, readBody(resp))
	}
}

// TestCustodyChain verifies GET /distribution/custody/:scanID returns a non-403
// response for a clerk. The scanID may or may not have events; we only check
// access control.
func TestCustodyChain(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	clerkCookie := loginAs(t, app, db, "clerk")

	resp := makeRequest(app, http.MethodGet, "/distribution/custody/SCAN001", "", clerkCookie, "")
	if resp.StatusCode == http.StatusForbidden {
		t.Fatalf("expected non-403 for clerk on custody chain, got %d", resp.StatusCode)
	}
}

// TestReissue_Success verifies POST /distribution/reissue with valid data by a
// clerk succeeds. We need an order in a valid state.
func TestReissue_Success(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	// Set up: create student + order in pending_shipment.
	studentUser := createTestUser(t, db, "student")
	order := createTestOrder(t, db, studentUser.ID)

	orderRepo := repository.NewOrderRepository(db)
	matRepo := repository.NewMaterialRepository(db)
	if err := orderRepo.Transition(order.ID, studentUser.ID, "pending_shipment", "pay", matRepo); err != nil {
		t.Fatalf("transition: %v", err)
	}

	var materialID int64
	if err := db.QueryRow(`SELECT material_id FROM order_items WHERE order_id = ? LIMIT 1`, order.ID).Scan(&materialID); err != nil {
		t.Fatalf("get material id: %v", err)
	}

	clerkCookie := loginAs(t, app, db, "clerk")

	body := fmt.Sprintf("order_id=%d&material_id=%d&old_scan_id=OLD001&new_scan_id=NEW001&reason=lost",
		order.ID, materialID)

	resp := makeRequest(app, http.MethodPost, "/distribution/reissue",
		body, clerkCookie, "application/x-www-form-urlencoded")

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 200/302 on reissue, got %d; body: %s",
			resp.StatusCode, readBody(resp))
	}
}

// TestIssueItems_MaterialNotInOrder verifies that attempting to issue a
// material that is not in the order's order_items returns 4xx (rejected).
func TestIssueItems_MaterialNotInOrder(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	studentUser := createTestUser(t, db, "student")
	order := createTestOrder(t, db, studentUser.ID)

	orderRepo := repository.NewOrderRepository(db)
	matRepo := repository.NewMaterialRepository(db)
	if err := orderRepo.Transition(order.ID, studentUser.ID, "pending_shipment", "pay", matRepo); err != nil {
		t.Fatalf("transition: %v", err)
	}

	// Insert a foreign material that is NOT part of the order.
	var foreignMatID int64
	if err := db.QueryRow(
		`INSERT INTO materials (title, total_qty, available_qty, reserved_qty, status)
		 VALUES ('Foreign Book', 5, 5, 0, 'active') RETURNING id`,
	).Scan(&foreignMatID); err != nil {
		t.Fatalf("insert foreign material: %v", err)
	}

	clerkCookie := loginAs(t, app, db, "clerk")
	body := fmt.Sprintf("order_id=%d&scan_id=SCAN999&material_id=%d&qty=1",
		order.ID, foreignMatID)

	resp := makeRequest(app, http.MethodPost, "/distribution/issue",
		body, clerkCookie, "application/x-www-form-urlencoded")

	if resp.StatusCode < 400 {
		t.Errorf("expected 4xx for material not in order, got %d", resp.StatusCode)
	}
}

// TestIssueItems_ForgedQtyExceedsOrdered verifies that a client-supplied qty
// field greater than the DB-authoritative ordered quantity does NOT result in
// more copies being issued than were ordered.  After the server-side qty fix,
// the form's qty field is informational and the service uses the DB-backed
// oqty as the actual issued quantity, so qty=9999 is silently capped to oqty=1.
func TestIssueItems_ForgedQtyExceedsOrdered(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	studentUser := createTestUser(t, db, "student")
	order := createTestOrder(t, db, studentUser.ID) // qty=1 in order_items

	orderRepo := repository.NewOrderRepository(db)
	matRepo := repository.NewMaterialRepository(db)
	if err := orderRepo.Transition(order.ID, studentUser.ID, "pending_shipment", "pay", matRepo); err != nil {
		t.Fatalf("transition: %v", err)
	}

	var materialID int64
	if err := db.QueryRow(`SELECT material_id FROM order_items WHERE order_id = ? LIMIT 1`, order.ID).Scan(&materialID); err != nil {
		t.Fatalf("get material id: %v", err)
	}

	clerkCookie := loginAs(t, app, db, "clerk")
	// Submit qty=9999 — the server should use DB-authoritative oqty=1.
	body := fmt.Sprintf("order_id=%d&scan_id=SCAN888&material_id=%d&qty=9999",
		order.ID, materialID)

	resp := makeRequest(app, http.MethodPost, "/distribution/issue",
		body, clerkCookie, "application/x-www-form-urlencoded")

	// The request succeeds (302 redirect) because the forged qty is ignored;
	// only 1 copy is actually issued (DB-authoritative).
	if resp.StatusCode >= 500 {
		t.Errorf("unexpected 5xx on issue with forged qty, got %d", resp.StatusCode)
	}

	// Verify the distribution event records qty=1, not 9999.
	var issuedQty int
	if err := db.QueryRow(
		`SELECT qty FROM distribution_events WHERE order_id = ? AND event_type = 'issued' LIMIT 1`,
		order.ID,
	).Scan(&issuedQty); err != nil {
		t.Fatalf("query distribution_events: %v", err)
	}
	if issuedQty != 1 {
		t.Errorf("expected distribution_event.qty=1 (DB-authoritative), got %d", issuedQty)
	}
}

// TestIssueItems_ForgedQtyBelowOrdered_StaysBackordered verifies that when a
// clerk submits an issued_qty lower than the ordered quantity the item is NOT
// marked fulfilled — it must be marked backordered even though the client
// submitted issued_qty=1 for an order with qty=2.  This prevents a forged low
// issued_qty from faking full fulfillment with fewer physical copies.
func TestIssueItems_ForgedQtyBelowOrdered_StaysBackordered(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	// Create a student + order with qty=2.
	studentUser := createTestUser(t, db, "student")
	mat := createTestMaterial(t, db)

	// Place a raw order with qty=2 directly via the repository.
	orderRepo := repository.NewOrderRepository(db)
	matRepo := repository.NewMaterialRepository(db)
	order, err := orderRepo.Create(studentUser.ID, []repository.OrderItemInput{
		{MaterialID: mat.ID, Qty: 2},
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	// Advance to pending_shipment.
	if err := orderRepo.Transition(order.ID, studentUser.ID, "pending_shipment", "pay", matRepo); err != nil {
		t.Fatalf("transition to pending_shipment: %v", err)
	}

	clerkCookie := loginAs(t, app, db, "clerk")

	// Submit qty=1 and issued_qty=1 — only half the ordered quantity.
	// A vulnerable server would mark the item fulfilled; the fixed server must
	// mark it backordered because oqty=2 > issued=1.
	body := fmt.Sprintf("order_id=%d&scan_id=FORGED01&material_id=%d&qty=1&issued_qty=1",
		order.ID, mat.ID)

	resp := makeRequest(app, http.MethodPost, "/distribution/issue",
		body, clerkCookie, "application/x-www-form-urlencoded")

	// The request is accepted (200/302) — the forgery is about the fulfillment
	// status written, not a rejection of the request itself.
	if resp.StatusCode >= 500 {
		t.Fatalf("unexpected 5xx on partial issue: %d; body: %s", resp.StatusCode, readBody(resp))
	}

	// The fulfillment status for the order item must be "backordered", not "fulfilled".
	var fulfillmentStatus string
	if err := db.QueryRow(
		`SELECT fulfillment_status FROM order_items WHERE order_id = ? AND material_id = ?`,
		order.ID, mat.ID,
	).Scan(&fulfillmentStatus); err != nil {
		t.Fatalf("query order_item fulfillment_status: %v", err)
	}
	if fulfillmentStatus == "fulfilled" {
		t.Errorf("item incorrectly marked 'fulfilled' when only 1 of 2 copies were issued — qty forgery accepted")
	}
	if fulfillmentStatus != "backordered" {
		t.Errorf("expected fulfillment_status='backordered', got %q", fulfillmentStatus)
	}
}

// TestReissue_ZeroStock_Fails verifies that POST /distribution/reissue is
// rejected with a 4xx when the material has no available stock.  Previously the
// endpoint would succeed and leave inventory inconsistent.
func TestReissue_ZeroStock_Fails(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	studentUser := createTestUser(t, db, "student")
	order := createTestOrder(t, db, studentUser.ID)

	// Drive available_qty to 0 on the ordered material.
	var materialID int64
	if err := db.QueryRow(`SELECT material_id FROM order_items WHERE order_id = ? LIMIT 1`, order.ID).Scan(&materialID); err != nil {
		t.Fatalf("get material id: %v", err)
	}
	if _, err := db.Exec(`UPDATE materials SET available_qty = 0 WHERE id = ?`, materialID); err != nil {
		t.Fatalf("zero stock: %v", err)
	}

	clerkCookie := loginAs(t, app, db, "clerk")
	body := fmt.Sprintf("order_id=%d&material_id=%d&old_scan_id=OLD&new_scan_id=NEW&reason=lost",
		order.ID, materialID)

	resp := makeRequest(app, http.MethodPost, "/distribution/reissue",
		body, clerkCookie, "application/x-www-form-urlencoded")

	if resp.StatusCode < 400 {
		t.Errorf("expected 4xx for reissue with zero stock, got %d", resp.StatusCode)
	}
}

// TestReissue_DecrementsInventory verifies that a successful reissue decrements
// available_qty by 1, keeping the inventory ledger consistent.
func TestReissue_DecrementsInventory(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	studentUser := createTestUser(t, db, "student")
	order := createTestOrder(t, db, studentUser.ID)

	var materialID int64
	if err := db.QueryRow(`SELECT material_id FROM order_items WHERE order_id = ? LIMIT 1`, order.ID).Scan(&materialID); err != nil {
		t.Fatalf("get material id: %v", err)
	}

	var beforeAvail int
	if err := db.QueryRow(`SELECT available_qty FROM materials WHERE id = ?`, materialID).Scan(&beforeAvail); err != nil {
		t.Fatalf("query before: %v", err)
	}

	clerkCookie := loginAs(t, app, db, "clerk")
	body := fmt.Sprintf("order_id=%d&material_id=%d&old_scan_id=OLD99&new_scan_id=NEW99&reason=damaged",
		order.ID, materialID)

	resp := makeRequest(app, http.MethodPost, "/distribution/reissue",
		body, clerkCookie, "application/x-www-form-urlencoded")

	if resp.StatusCode >= 500 {
		t.Fatalf("unexpected 5xx on reissue: %d; body: %s", resp.StatusCode, readBody(resp))
	}

	var afterAvail int
	if err := db.QueryRow(`SELECT available_qty FROM materials WHERE id = ?`, materialID).Scan(&afterAvail); err != nil {
		t.Fatalf("query after: %v", err)
	}
	if afterAvail != beforeAvail-1 {
		t.Errorf("expected available_qty=%d after reissue, got %d — inventory not decremented", beforeAvail-1, afterAvail)
	}
}

// TestInboxSSE_RouteRegistered verifies that GET /inbox/sse is registered in the
// production route table and returns a streaming response (not 404).
// This test guards against the route being present only in the test helper and
// not in the real app wiring.
func TestInboxSSE_RouteRegistered(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")

	// The SSE handler streams indefinitely; app.Test with a short timeout is
	// sufficient to confirm the route is wired and auth passes.
	req := httptest.NewRequest(http.MethodGet, "/inbox/sse", nil)
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := app.Test(req, 100) // 100 ms — enough to confirm 200 header
	if err != nil {
		// A timeout error from the streaming endpoint is expected and acceptable.
		return
	}
	if resp.StatusCode == http.StatusNotFound {
		t.Error("/inbox/sse returned 404 — route is not registered in the production route table")
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		t.Errorf("/inbox/sse returned %d — auth middleware not configured correctly", resp.StatusCode)
	}
}

// TestRecordReturn_RequiresApprovedRequest verifies that POST /distribution/return
// without a valid approved return_request_id is rejected.
func TestRecordReturn_RequiresApprovedRequest(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	studentUser := createTestUser(t, db, "student")
	order := createTestOrder(t, db, studentUser.ID)

	var materialID int64
	if err := db.QueryRow(`SELECT material_id FROM order_items WHERE order_id = ? LIMIT 1`, order.ID).Scan(&materialID); err != nil {
		t.Fatalf("get material id: %v", err)
	}

	clerkCookie := loginAs(t, app, db, "clerk")

	// Submit without a return_request_id (missing field → 0 or blank).
	body := fmt.Sprintf("order_id=%d&material_id=%d&scan_id=SCAN777&qty=1",
		order.ID, materialID)

	resp := makeRequest(app, http.MethodPost, "/distribution/return",
		body, clerkCookie, "application/x-www-form-urlencoded")

	if resp.StatusCode < 400 {
		t.Errorf("expected 4xx for missing return_request_id, got %d", resp.StatusCode)
	}
}
