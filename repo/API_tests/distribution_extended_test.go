package api_tests

import (
	"fmt"
	"net/http"
	"testing"

	"w2t86/internal/models"
	"w2t86/internal/repository"
)

// distribution_extended_test.go covers distribution endpoints not tested elsewhere:
//   POST /distribution/return    — record physical return
//   POST /distribution/exchange  — record exchange
//   GET  /distribution/reissue   — reissue form page
//   POST /distribution/reissue   — reissue a copy
//   GET  /distribution/ledger/search  — filtered ledger partial
//   GET  /distribution/custody/:scanID — custody chain timeline

// ---------------------------------------------------------------------------
// POST /distribution/return
// ---------------------------------------------------------------------------

// TestDistribution_RecordReturn_MissingFields rejects a request without required fields.
func TestDistribution_RecordReturn_MissingFields(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "clerk")
	// Send empty body — handler must return 4xx for missing/invalid fields.
	resp := makeRequest(app, http.MethodPost, "/distribution/return",
		"", cookie, "application/x-www-form-urlencoded")
	if resp.StatusCode/100 == 2 || resp.StatusCode == http.StatusFound {
		t.Fatalf("expected 4xx for missing return fields, got %d; body: %s",
			resp.StatusCode, readBody(resp))
	}
}

// TestDistribution_RecordReturn_StudentForbidden returns 403.
func TestDistribution_RecordReturn_StudentForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodPost, "/distribution/return",
		"order_id=1&material_id=1&return_request_id=1&scan_id=S1&qty=1",
		cookie, "application/x-www-form-urlencoded")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for student on distribution/return, got %d", resp.StatusCode)
	}
}

// TestDistribution_RecordReturn_WithApprovedRequest issues an item and records its return.
func TestDistribution_RecordReturn_WithApprovedRequest(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	student := createUser(t, db, "student")
	mat := createMaterial(t, db)

	orderRepo := repository.NewOrderRepository(db)
	matRepo := repository.NewMaterialRepository(db)
	distRepo := repository.NewDistributionRepository(db)

	order, err := orderRepo.Create(student.ID, []repository.OrderItemInput{
		{MaterialID: mat.ID, Qty: 1},
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	_ = orderRepo.Transition(order.ID, student.ID, "paid", "pending_payment", matRepo)
	_ = orderRepo.Transition(order.ID, student.ID, "pending_shipment", "paid", matRepo)

	// Issue a copy via RecordEvent.
	scanID := fmt.Sprintf("SCAN-RET-%d", student.ID)
	clerk := createUser(t, db, "clerk")
	scanStr := scanID
	clerkIDVal := clerk.ID
	orderIDVal := order.ID
	if _, err := distRepo.RecordEvent(&models.DistributionEvent{
		OrderID:    &orderIDVal,
		MaterialID: mat.ID,
		Qty:        1,
		EventType:  "issue",
		ScanID:     &scanStr,
		ActorID:    &clerkIDVal,
	}); err != nil {
		t.Fatalf("issue item: %v", err)
	}

	// Create an approved return request.
	var returnID int64
	err = db.QueryRow(
		`INSERT INTO return_requests (order_id, user_id, type, reason, status)
		 VALUES (?, ?, 'return', 'test', 'approved')
		 RETURNING id`,
		order.ID, student.ID,
	).Scan(&returnID)
	if err != nil {
		t.Fatalf("create return request: %v", err)
	}

	cookie := loginAs(t, app, db, "clerk")
	body := fmt.Sprintf(
		"order_id=%d&material_id=%d&return_request_id=%d&scan_id=%s&qty=1",
		order.ID, mat.ID, returnID, scanID,
	)
	resp := makeRequest(app, http.MethodPost, "/distribution/return",
		body, cookie, "application/x-www-form-urlencoded", htmx())
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 2xx/302 for valid return, got %d; body: %s",
			resp.StatusCode, readBody(resp))
	}
}

// ---------------------------------------------------------------------------
// POST /distribution/exchange
// ---------------------------------------------------------------------------

// TestDistribution_RecordExchange_MissingFields rejects missing required params.
func TestDistribution_RecordExchange_MissingFields(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "clerk")
	resp := makeRequest(app, http.MethodPost, "/distribution/exchange",
		"", cookie, "application/x-www-form-urlencoded")
	if resp.StatusCode/100 == 2 || resp.StatusCode == http.StatusFound {
		t.Fatalf("expected 4xx for missing exchange fields, got %d", resp.StatusCode)
	}
}

// TestDistribution_RecordExchange_StudentForbidden returns 403.
func TestDistribution_RecordExchange_StudentForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodPost, "/distribution/exchange",
		"order_id=1&old_material_id=1&new_material_id=2&return_request_id=1&scan_id=S1&qty=1",
		cookie, "application/x-www-form-urlencoded")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for student on distribution/exchange, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// GET /distribution/reissue — reissue form
// ---------------------------------------------------------------------------

// TestDistribution_ReissueForm_ClerkAllowed renders the reissue form for a clerk.
func TestDistribution_ReissueForm_ClerkAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "clerk")
	resp := makeRequest(app, http.MethodGet, "/distribution/reissue", "", cookie, "")
	if resp.StatusCode/100 != 2 {
		t.Fatalf("expected 2xx for reissue form, got %d; body: %s", resp.StatusCode, readBody(resp))
	}
}

// TestDistribution_ReissueForm_StudentForbidden returns 403.
func TestDistribution_ReissueForm_StudentForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodGet, "/distribution/reissue", "", cookie, "")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for student on reissue form, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// POST /distribution/reissue
// ---------------------------------------------------------------------------

// TestDistribution_Reissue_MissingFields rejects empty body.
func TestDistribution_Reissue_MissingFields(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "clerk")
	resp := makeRequest(app, http.MethodPost, "/distribution/reissue",
		"", cookie, "application/x-www-form-urlencoded")
	if resp.StatusCode/100 == 2 || resp.StatusCode == http.StatusFound {
		t.Fatalf("expected 4xx for missing reissue fields, got %d", resp.StatusCode)
	}
}

// TestDistribution_Reissue_ValidFlow issues an item then reissues it.
func TestDistribution_Reissue_ValidFlow(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	student := createUser(t, db, "student")
	mat := createMaterial(t, db)

	orderRepo := repository.NewOrderRepository(db)
	matRepo := repository.NewMaterialRepository(db)
	distRepo := repository.NewDistributionRepository(db)
	clerkUser := createUser(t, db, "clerk")

	order, err := orderRepo.Create(student.ID, []repository.OrderItemInput{
		{MaterialID: mat.ID, Qty: 1},
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	_ = orderRepo.Transition(order.ID, student.ID, "paid", "pending_payment", matRepo)
	_ = orderRepo.Transition(order.ID, student.ID, "pending_shipment", "paid", matRepo)

	oldScan := fmt.Sprintf("OLD-SCAN-%d", student.ID)
	oldScanStr := oldScan
	clerkIDV := clerkUser.ID
	orderIDV := order.ID
	if _, err := distRepo.RecordEvent(&models.DistributionEvent{
		OrderID:    &orderIDV,
		MaterialID: mat.ID,
		Qty:        1,
		EventType:  "issue",
		ScanID:     &oldScanStr,
		ActorID:    &clerkIDV,
	}); err != nil {
		t.Fatalf("issue item: %v", err)
	}

	cookie := loginAs(t, app, db, "clerk")
	newScan := fmt.Sprintf("NEW-SCAN-%d", student.ID)
	body := fmt.Sprintf(
		"order_id=%d&material_id=%d&old_scan_id=%s&new_scan_id=%s&reason=damaged",
		order.ID, mat.ID, oldScan, newScan,
	)
	resp := makeRequest(app, http.MethodPost, "/distribution/reissue",
		body, cookie, "application/x-www-form-urlencoded", htmx())
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 2xx/302 for valid reissue, got %d; body: %s",
			resp.StatusCode, readBody(resp))
	}
}

// ---------------------------------------------------------------------------
// GET /distribution/ledger/search
// ---------------------------------------------------------------------------

// TestDistribution_LedgerSearch_ClerkAllowed returns 2xx with empty query.
func TestDistribution_LedgerSearch_ClerkAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "clerk")
	resp := makeRequest(app, http.MethodGet, "/distribution/ledger/search", "", cookie, "", htmx())
	if resp.StatusCode/100 != 2 {
		t.Fatalf("expected 2xx for ledger search, got %d; body: %s", resp.StatusCode, readBody(resp))
	}
}

// TestDistribution_LedgerSearch_WithScanID returns 2xx for a scan_id filter.
func TestDistribution_LedgerSearch_WithScanID(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "clerk")
	resp := makeRequest(app, http.MethodGet, "/distribution/ledger/search?scan_id=SCAN-999", "", cookie, "")
	if resp.StatusCode/100 != 2 {
		t.Fatalf("expected 2xx for ledger search with scan_id, got %d", resp.StatusCode)
	}
}

// TestDistribution_LedgerSearch_StudentForbidden returns 403.
func TestDistribution_LedgerSearch_StudentForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodGet, "/distribution/ledger/search", "", cookie, "")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for student on ledger search, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// GET /distribution/custody/:scanID
// ---------------------------------------------------------------------------

// TestDistribution_CustodyChain_ExistingScan returns 2xx for a known scan ID.
func TestDistribution_CustodyChain_ExistingScan(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	student := createUser(t, db, "student")
	mat := createMaterial(t, db)

	orderRepo := repository.NewOrderRepository(db)
	matRepo := repository.NewMaterialRepository(db)
	distRepo := repository.NewDistributionRepository(db)
	clerkUser := createUser(t, db, "clerk")

	order, err := orderRepo.Create(student.ID, []repository.OrderItemInput{
		{MaterialID: mat.ID, Qty: 1},
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	_ = orderRepo.Transition(order.ID, student.ID, "paid", "pending_payment", matRepo)
	_ = orderRepo.Transition(order.ID, student.ID, "pending_shipment", "paid", matRepo)

	scanID := fmt.Sprintf("CUST-SCAN-%d", student.ID)
	scanIDStr := scanID
	clerkIDC := clerkUser.ID
	orderIDC := order.ID
	if _, err := distRepo.RecordEvent(&models.DistributionEvent{
		OrderID:    &orderIDC,
		MaterialID: mat.ID,
		Qty:        1,
		EventType:  "issue",
		ScanID:     &scanIDStr,
		ActorID:    &clerkIDC,
	}); err != nil {
		t.Fatalf("issue item: %v", err)
	}

	cookie := loginAs(t, app, db, "clerk")
	resp := makeRequest(app, http.MethodGet,
		fmt.Sprintf("/distribution/custody/%s", scanID), "", cookie, "")
	if resp.StatusCode/100 != 2 {
		t.Fatalf("expected 2xx for custody chain, got %d; body: %s", resp.StatusCode, readBody(resp))
	}
}

// TestDistribution_CustodyChain_UnknownScan returns 2xx (empty timeline or 404).
func TestDistribution_CustodyChain_UnknownScan(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "clerk")
	resp := makeRequest(app, http.MethodGet, "/distribution/custody/UNKNOWN-SCAN-XYZ", "", cookie, "")
	// Handler may return 404 or empty page — both acceptable.
	if resp.StatusCode == http.StatusInternalServerError {
		t.Fatalf("server error for unknown custody scan: %s", readBody(resp))
	}
}

// TestDistribution_CustodyChain_StudentForbidden returns 403.
func TestDistribution_CustodyChain_StudentForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodGet, "/distribution/custody/SCAN-1", "", cookie, "")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for student on custody chain, got %d", resp.StatusCode)
	}
}
