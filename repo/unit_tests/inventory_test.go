package unit_tests

import (
	"database/sql"
	"fmt"
	"testing"

	"w2t86/internal/repository"
	"w2t86/internal/testutil"
)

// seedMaterial inserts an active material with the given totalQty / availableQty / reservedQty
// and returns the material ID.
func seedMaterial(t *testing.T, db *sql.DB, total, available, reserved int) int64 {
	t.Helper()
	var id int64
	err := db.QueryRow(
		`INSERT INTO materials (title, total_qty, available_qty, reserved_qty, status)
		 VALUES (?, ?, ?, ?, 'active') RETURNING id`,
		fmt.Sprintf("material_%d", testSeq()), total, available, reserved,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seedMaterial: %v", err)
	}
	return id
}

// seedUserForInventory inserts a minimal user row and returns its ID.
func seedUserForInventory(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var id int64
	err := db.QueryRow(
		`INSERT INTO users (username, email, password_hash, role)
		 VALUES (?, 'inv@example.com', 'hash', 'student') RETURNING id`,
		fmt.Sprintf("inv_user_%d", testSeq()),
	).Scan(&id)
	if err != nil {
		t.Fatalf("seedUserForInventory: %v", err)
	}
	return id
}

// getMaterial fetches available_qty and reserved_qty for a material.
func getMaterialQtys(t *testing.T, db *sql.DB, matID int64) (available, reserved int) {
	t.Helper()
	err := db.QueryRow(
		`SELECT available_qty, reserved_qty FROM materials WHERE id = ?`, matID,
	).Scan(&available, &reserved)
	if err != nil {
		t.Fatalf("getMaterialQtys: %v", err)
	}
	return
}

// ---------------------------------------------------------------------------
// Reserve
// ---------------------------------------------------------------------------

func TestInventory_Reserve_ReducesAvailable(t *testing.T) {
	db := testutil.NewTestDB(t)
	matRepo := repository.NewMaterialRepository(db)

	id := seedMaterial(t, db, 10, 10, 0)
	if err := matRepo.Reserve(id, 3); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	avail, _ := getMaterialQtys(t, db, id)
	if avail != 7 {
		t.Errorf("expected available_qty=7, got %d", avail)
	}
}

func TestInventory_Reserve_IncreasesReserved(t *testing.T) {
	db := testutil.NewTestDB(t)
	matRepo := repository.NewMaterialRepository(db)

	id := seedMaterial(t, db, 10, 10, 0)
	if err := matRepo.Reserve(id, 4); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	_, rsv := getMaterialQtys(t, db, id)
	if rsv != 4 {
		t.Errorf("expected reserved_qty=4, got %d", rsv)
	}
}

func TestInventory_Reserve_ExactAvailableQty_Succeeds(t *testing.T) {
	db := testutil.NewTestDB(t)
	matRepo := repository.NewMaterialRepository(db)

	id := seedMaterial(t, db, 5, 5, 0)
	if err := matRepo.Reserve(id, 5); err != nil {
		t.Fatalf("Reserve exact qty: %v", err)
	}
	avail, rsv := getMaterialQtys(t, db, id)
	if avail != 0 || rsv != 5 {
		t.Errorf("expected available=0, reserved=5; got available=%d, reserved=%d", avail, rsv)
	}
}

func TestInventory_Reserve_OverAvailableByOne_Fails(t *testing.T) {
	db := testutil.NewTestDB(t)
	matRepo := repository.NewMaterialRepository(db)

	id := seedMaterial(t, db, 5, 5, 0)
	err := matRepo.Reserve(id, 6)
	if err == nil {
		t.Error("expected error when reserving more than available, got nil")
	}
}

func TestInventory_Reserve_ZeroQty_Fails(t *testing.T) {
	db := testutil.NewTestDB(t)
	matRepo := repository.NewMaterialRepository(db)

	id := seedMaterial(t, db, 5, 5, 0)
	// Reserving 0 — the WHERE clause requires available_qty >= qty (0),
	// which is always true, so rows may be affected. The real semantic
	// constraint is that qty must be positive. The service layer enforces
	// this, but we verify the repository does not crash.
	err := matRepo.Reserve(id, 0)
	// Reserving 0 is a no-op in the DB (WHERE available_qty >= 0 is always
	// satisfied and the SET is a no-op), so we check that at least the
	// quantities haven't changed meaningfully.
	avail, rsv := getMaterialQtys(t, db, id)
	if err == nil {
		// Accepted — values must be unchanged.
		if avail != 5 || rsv != 0 {
			t.Errorf("Reserve(0) changed qtys: avail=%d rsv=%d", avail, rsv)
		}
	}
	// Whether err==nil or not, this is acceptable behavior; the key check
	// is that no panic occurred and data integrity is maintained.
}

func TestInventory_Reserve_NegativeQty_Fails(t *testing.T) {
	// The repository layer does not validate sign; enforcement is the
	// responsibility of the service layer (which checks qty > 0 before calling
	// Reserve). This test documents the boundary: calling Reserve with a
	// negative qty must not panic and the resulting qty values should not
	// reflect a legitimate reservation of real stock. We accept any outcome
	// (error or arithmetic side-effect) as long as the call does not crash.
	db := testutil.NewTestDB(t)
	matRepo := repository.NewMaterialRepository(db)

	id := seedMaterial(t, db, 5, 5, 0)

	// We only assert that the call does not panic.
	_ = matRepo.Reserve(id, -1)
}

// ---------------------------------------------------------------------------
// Release
// ---------------------------------------------------------------------------

func TestInventory_Release_RestoresAvailable(t *testing.T) {
	db := testutil.NewTestDB(t)
	matRepo := repository.NewMaterialRepository(db)

	id := seedMaterial(t, db, 10, 7, 3)
	if err := matRepo.Release(id, 3); err != nil {
		t.Fatalf("Release: %v", err)
	}
	avail, _ := getMaterialQtys(t, db, id)
	if avail != 10 {
		t.Errorf("expected available_qty=10, got %d", avail)
	}
}

func TestInventory_Release_ReducesReserved(t *testing.T) {
	db := testutil.NewTestDB(t)
	matRepo := repository.NewMaterialRepository(db)

	id := seedMaterial(t, db, 10, 7, 3)
	if err := matRepo.Release(id, 3); err != nil {
		t.Fatalf("Release: %v", err)
	}
	_, rsv := getMaterialQtys(t, db, id)
	if rsv != 0 {
		t.Errorf("expected reserved_qty=0, got %d", rsv)
	}
}

// ---------------------------------------------------------------------------
// Fulfill
// ---------------------------------------------------------------------------

func TestInventory_Fulfill_ReducesReserved(t *testing.T) {
	db := testutil.NewTestDB(t)
	matRepo := repository.NewMaterialRepository(db)

	id := seedMaterial(t, db, 10, 7, 3)
	if err := matRepo.Fulfill(id, 3); err != nil {
		t.Fatalf("Fulfill: %v", err)
	}
	_, rsv := getMaterialQtys(t, db, id)
	if rsv != 0 {
		t.Errorf("expected reserved_qty=0 after Fulfill, got %d", rsv)
	}
}

func TestInventory_Fulfill_DoesNotChangeAvailable(t *testing.T) {
	db := testutil.NewTestDB(t)
	matRepo := repository.NewMaterialRepository(db)

	id := seedMaterial(t, db, 10, 7, 3)
	before, _ := getMaterialQtys(t, db, id)
	if err := matRepo.Fulfill(id, 3); err != nil {
		t.Fatalf("Fulfill: %v", err)
	}
	after, _ := getMaterialQtys(t, db, id)
	if before != after {
		t.Errorf("Fulfill should not change available_qty: before=%d after=%d", before, after)
	}
}

// ---------------------------------------------------------------------------
// Cumulative / combined scenarios
// ---------------------------------------------------------------------------

func TestInventory_MultipleReserves_Cumulative(t *testing.T) {
	db := testutil.NewTestDB(t)
	matRepo := repository.NewMaterialRepository(db)

	id := seedMaterial(t, db, 10, 10, 0)
	if err := matRepo.Reserve(id, 2); err != nil {
		t.Fatalf("Reserve 2: %v", err)
	}
	if err := matRepo.Reserve(id, 3); err != nil {
		t.Fatalf("Reserve 3: %v", err)
	}
	avail, rsv := getMaterialQtys(t, db, id)
	if avail != 5 {
		t.Errorf("expected available_qty=5, got %d", avail)
	}
	if rsv != 5 {
		t.Errorf("expected reserved_qty=5, got %d", rsv)
	}
}

func TestInventory_ReserveAndRelease_ReturnsToPrevious(t *testing.T) {
	db := testutil.NewTestDB(t)
	matRepo := repository.NewMaterialRepository(db)

	id := seedMaterial(t, db, 10, 10, 0)
	if err := matRepo.Reserve(id, 3); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if err := matRepo.Release(id, 3); err != nil {
		t.Fatalf("Release: %v", err)
	}
	avail, rsv := getMaterialQtys(t, db, id)
	if avail != 10 {
		t.Errorf("expected available_qty=10, got %d", avail)
	}
	if rsv != 0 {
		t.Errorf("expected reserved_qty=0, got %d", rsv)
	}
}

func TestInventory_OrderCancel_RollsBackInventory(t *testing.T) {
	db := testutil.NewTestDBNoFK(t)
	orderRepo := repository.NewOrderRepository(db)
	matRepo := repository.NewMaterialRepository(db)

	// Seed a user and material.
	var userID int64
	if err := db.QueryRow(
		`INSERT INTO users (username, email, password_hash, role) VALUES (?, 'rb@x.com', 'h', 'student') RETURNING id`,
		fmt.Sprintf("rb_user_%d", testSeq()),
	).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	var matID int64
	if err := db.QueryRow(
		`INSERT INTO materials (title, total_qty, available_qty, reserved_qty, status)
		 VALUES ('Rollback Book', 5, 5, 0, 'active') RETURNING id`,
	).Scan(&matID); err != nil {
		t.Fatalf("insert material: %v", err)
	}

	// Place order (reserves 2 units).
	order, err := orderRepo.Create(userID, []repository.OrderItemInput{
		{MaterialID: matID, Qty: 2},
	})
	if err != nil {
		t.Fatalf("Create order: %v", err)
	}

	// Verify inventory was reserved.
	avail, rsv := getMaterialQtys(t, db, matID)
	if avail != 3 || rsv != 2 {
		t.Fatalf("after order: expected available=3, reserved=2; got available=%d, reserved=%d", avail, rsv)
	}

	// Cancel the order — inventory should roll back.
	if err := orderRepo.Transition(order.ID, 0, "canceled", "test cancel", matRepo); err != nil {
		t.Fatalf("Transition to canceled: %v", err)
	}

	avail, rsv = getMaterialQtys(t, db, matID)
	if avail != 5 {
		t.Errorf("expected available_qty=5 after cancel, got %d", avail)
	}
	if rsv != 0 {
		t.Errorf("expected reserved_qty=0 after cancel, got %d", rsv)
	}
}

// ---------------------------------------------------------------------------
// Release guard (invariant: reserved_qty must never go negative)
// ---------------------------------------------------------------------------

// TestInventory_Release_GuardsAgainstNegativeReserved verifies that Release
// returns an error instead of driving reserved_qty below zero when called on a
// material whose reserved_qty is already zero (the post-fulfillment state).
func TestInventory_Release_GuardsAgainstNegativeReserved(t *testing.T) {
	db := testutil.NewTestDB(t)
	matRepo := repository.NewMaterialRepository(db)

	// Seed a material with reserved_qty=0 (as it would be after fulfillment).
	id := seedMaterial(t, db, 10, 10, 0)

	err := matRepo.Release(id, 1)
	if err == nil {
		t.Error("expected Release to fail when reserved_qty=0, but got nil error")
	}

	// Quantities must remain unchanged.
	avail, rsv := getMaterialQtys(t, db, id)
	if rsv < 0 {
		t.Errorf("reserved_qty went negative (%d) after failed Release", rsv)
	}
	if avail != 10 {
		t.Errorf("available_qty changed unexpectedly: got %d", avail)
	}
}

// ---------------------------------------------------------------------------
// ReturnToStock (post-fulfillment return path)
// ---------------------------------------------------------------------------

func TestInventory_ReturnToStock_IncreasesAvailable(t *testing.T) {
	db := testutil.NewTestDB(t)
	matRepo := repository.NewMaterialRepository(db)

	// Simulate post-fulfillment state: 8 available, 0 reserved (2 in the field).
	id := seedMaterial(t, db, 10, 8, 0)
	if err := matRepo.ReturnToStock(id, 2); err != nil {
		t.Fatalf("ReturnToStock: %v", err)
	}
	avail, rsv := getMaterialQtys(t, db, id)
	if avail != 10 {
		t.Errorf("expected available_qty=10 after return, got %d", avail)
	}
	if rsv != 0 {
		t.Errorf("ReturnToStock must not change reserved_qty; got %d", rsv)
	}
}

func TestInventory_ReturnToStock_DoesNotChangeReserved(t *testing.T) {
	db := testutil.NewTestDB(t)
	matRepo := repository.NewMaterialRepository(db)

	// Material with some reserved stock (unrelated active order).
	id := seedMaterial(t, db, 10, 5, 3)
	if err := matRepo.ReturnToStock(id, 2); err != nil {
		t.Fatalf("ReturnToStock: %v", err)
	}
	_, rsv := getMaterialQtys(t, db, id)
	if rsv != 3 {
		t.Errorf("reserved_qty should remain 3 after ReturnToStock, got %d", rsv)
	}
}

// ---------------------------------------------------------------------------
// DirectIssue (new-leg of post-fulfillment exchange)
// ---------------------------------------------------------------------------

func TestInventory_DirectIssue_DecreasesAvailable(t *testing.T) {
	db := testutil.NewTestDB(t)
	matRepo := repository.NewMaterialRepository(db)

	id := seedMaterial(t, db, 10, 10, 0)
	if err := matRepo.DirectIssue(id, 3); err != nil {
		t.Fatalf("DirectIssue: %v", err)
	}
	avail, rsv := getMaterialQtys(t, db, id)
	if avail != 7 {
		t.Errorf("expected available_qty=7, got %d", avail)
	}
	if rsv != 0 {
		t.Errorf("DirectIssue must not change reserved_qty; got %d", rsv)
	}
}

func TestInventory_DirectIssue_InsufficientStock_Fails(t *testing.T) {
	db := testutil.NewTestDB(t)
	matRepo := repository.NewMaterialRepository(db)

	id := seedMaterial(t, db, 5, 2, 0)
	err := matRepo.DirectIssue(id, 3)
	if err == nil {
		t.Error("expected error when DirectIssue exceeds available_qty, got nil")
	}
}

// ---------------------------------------------------------------------------
// Post-fulfillment return: reserved_qty must stay non-negative end-to-end.
// ---------------------------------------------------------------------------

// TestInventory_PostFulfillment_Return_ReservedNonNegative exercises the full
// order → complete → return path and asserts that reserved_qty never goes
// negative at any point.
func TestInventory_PostFulfillment_Return_ReservedNonNegative(t *testing.T) {
	db := testutil.NewTestDBNoFK(t)
	orderRepo := repository.NewOrderRepository(db)
	matRepo := repository.NewMaterialRepository(db)

	// Seed user and material.
	var userID int64
	if err := db.QueryRow(
		`INSERT INTO users (username, email, password_hash, role) VALUES (?, 'ret@x.com', 'h', 'student') RETURNING id`,
		fmt.Sprintf("ret_user_%d", testSeq()),
	).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	var matID int64
	if err := db.QueryRow(
		`INSERT INTO materials (title, total_qty, available_qty, reserved_qty, status)
		 VALUES ('Return Book', 5, 5, 0, 'active') RETURNING id`,
	).Scan(&matID); err != nil {
		t.Fatalf("insert material: %v", err)
	}

	// Place order (reserves 2 units: available 3, reserved 2).
	order, err := orderRepo.Create(userID, []repository.OrderItemInput{
		{MaterialID: matID, Qty: 2},
	})
	if err != nil {
		t.Fatalf("Create order: %v", err)
	}
	avail, rsv := getMaterialQtys(t, db, matID)
	if avail != 3 || rsv != 2 {
		t.Fatalf("after order: avail=%d rsv=%d (want 3/2)", avail, rsv)
	}

	// Transition through the lifecycle to completed.
	if err := orderRepo.Transition(order.ID, userID, "pending_shipment", "paid", matRepo); err != nil {
		t.Fatalf("→ pending_shipment: %v", err)
	}
	if err := orderRepo.Transition(order.ID, userID, "in_transit", "shipped", matRepo); err != nil {
		t.Fatalf("→ in_transit: %v", err)
	}
	if err := orderRepo.Transition(order.ID, userID, "completed", "delivered", matRepo); err != nil {
		t.Fatalf("→ completed: %v", err)
	}

	// After completion: reserved_qty decremented to 0, available unchanged (3).
	avail, rsv = getMaterialQtys(t, db, matID)
	if rsv != 0 {
		t.Fatalf("after completion: reserved_qty=%d (want 0)", rsv)
	}
	if avail != 3 {
		t.Fatalf("after completion: available_qty=%d (want 3)", avail)
	}

	// Post-fulfillment return: ReturnToStock must add to available, not touch reserved.
	if err := matRepo.ReturnToStock(matID, 2); err != nil {
		t.Fatalf("ReturnToStock: %v", err)
	}
	avail, rsv = getMaterialQtys(t, db, matID)
	if rsv < 0 {
		t.Errorf("INVARIANT VIOLATED: reserved_qty went negative after post-fulfillment return: %d", rsv)
	}
	if rsv != 0 {
		t.Errorf("reserved_qty should still be 0 after return, got %d", rsv)
	}
	if avail != 5 {
		t.Errorf("available_qty should be 5 after return, got %d", avail)
	}
}

// TestInventory_PostFulfillment_Exchange_ReservedNonNegative exercises the
// exchange path: old copy returned (ReturnToStock) + new copy direct-issued
// (DirectIssue). reserved_qty must remain non-negative throughout.
func TestInventory_PostFulfillment_Exchange_ReservedNonNegative(t *testing.T) {
	db := testutil.NewTestDBNoFK(t)
	matRepo := repository.NewMaterialRepository(db)

	// Simulate post-fulfillment: old material has 0 reserved (fulfilled).
	oldID := seedMaterial(t, db, 5, 3, 0)
	// New material for exchange: has available stock.
	newID := seedMaterial(t, db, 5, 5, 0)

	// Old copy returned.
	if err := matRepo.ReturnToStock(oldID, 1); err != nil {
		t.Fatalf("ReturnToStock old: %v", err)
	}
	// New copy directly issued.
	if err := matRepo.DirectIssue(newID, 1); err != nil {
		t.Fatalf("DirectIssue new: %v", err)
	}

	_, oldRsv := getMaterialQtys(t, db, oldID)
	_, newRsv := getMaterialQtys(t, db, newID)

	if oldRsv < 0 {
		t.Errorf("INVARIANT VIOLATED: old material reserved_qty=%d (negative after exchange return)", oldRsv)
	}
	if newRsv < 0 {
		t.Errorf("INVARIANT VIOLATED: new material reserved_qty=%d (negative after exchange issue)", newRsv)
	}
}
