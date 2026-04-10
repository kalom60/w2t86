package services

import (
	"database/sql"
	"errors"
	"fmt"

	"w2t86/internal/models"
	"w2t86/internal/observability"
	"w2t86/internal/repository"
)

// IssueItem identifies a single material line to be physically issued.
// IssuedQty is the number of copies the clerk is handing out right now; it may
// be less than the full ordered quantity, in which case a backorder is created
// for the remainder.  When IssuedQty is 0 it defaults to the full ordered qty.
type IssueItem struct {
	MaterialID int64
	Qty        int // total ordered quantity
	IssuedQty  int // quantity being issued in this operation (0 = full qty)
}

// DistributionService orchestrates all clerk-facing distribution operations:
// issuing, returning, exchanging, and reissuing physical copies.
type DistributionService struct {
	distRepo     *repository.DistributionRepository
	orderRepo    *repository.OrderRepository
	materialRepo *repository.MaterialRepository
}

// NewDistributionService wires the service to its three repositories.
func NewDistributionService(
	dr *repository.DistributionRepository,
	or *repository.OrderRepository,
	mr *repository.MaterialRepository,
) *DistributionService {
	return &DistributionService{
		distRepo:     dr,
		orderRepo:    or,
		materialRepo: mr,
	}
}

// ---------------------------------------------------------------
// Issue
// ---------------------------------------------------------------

// IssueItems records an "issued" distribution event for every item in the
// slice, marks the corresponding order_items as fulfilled, and — when all
// items for the order are fulfilled — advances the order status to in_transit.
//
// scanID is a barcode / copy identifier that binds the physical copy to the
// event.  It is shared across all items in a single issue batch.
func (s *DistributionService) IssueItems(orderID, actorID int64, scanID string, items []IssueItem) error {
	if len(items) == 0 {
		return errors.New("service: IssueItems: at least one item required")
	}

	order, err := s.orderRepo.GetByID(orderID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("service: IssueItems: order %d not found", orderID)
		}
		return fmt.Errorf("service: IssueItems: load order: %w", err)
	}
	if order.Status != "pending_shipment" && order.Status != "in_transit" {
		return fmt.Errorf("service: IssueItems: order %d is not ready to issue (status=%s)", orderID, order.Status)
	}

	scanIDPtr := &scanID
	actorIDPtr := &actorID
	orderIDPtr := &orderID

	// Load the authoritative order line items and build a lookup map.
	orderItems, err := s.orderRepo.GetItemsByOrderID(orderID)
	if err != nil {
		return fmt.Errorf("service: IssueItems: load order items: %w", err)
	}
	orderedQty := make(map[int64]int, len(orderItems))
	for _, oi := range orderItems {
		orderedQty[oi.MaterialID] = oi.Qty
	}

	for _, item := range items {
		// Reject materials that are not part of this order.
		oqty, ok := orderedQty[item.MaterialID]
		if !ok {
			return fmt.Errorf("service: IssueItems: material %d is not part of order %d", item.MaterialID, orderID)
		}

		issued := item.IssuedQty
		if issued <= 0 {
			// Default to the full DB-authoritative ordered quantity — never the
			// client-supplied item.Qty, which could be forged lower to fake
			// full fulfillment with fewer physical copies.
			issued = oqty
		}
		if issued <= 0 {
			return fmt.Errorf("service: IssueItems: qty must be positive for material %d", item.MaterialID)
		}
		// Issued quantity must not exceed the DB-authoritative ordered quantity.
		if issued > oqty {
			return fmt.Errorf("service: IssueItems: issued qty (%d) exceeds ordered qty (%d) for material %d",
				issued, oqty, item.MaterialID)
		}

		evt := &models.DistributionEvent{
			OrderID:     orderIDPtr,
			MaterialID:  item.MaterialID,
			Qty:         issued, // record only the quantity actually handed out
			EventType:   "issued",
			ScanID:      scanIDPtr,
			ActorID:     actorIDPtr,
			CustodyFrom: stringPtr("clerk"),
			CustodyTo:   stringPtr("student"),
		}
		if _, err := s.distRepo.RecordEvent(evt); err != nil {
			return fmt.Errorf("service: IssueItems: record event for material %d: %w", item.MaterialID, err)
		}
	}

	// Mark order items as fulfilled and potentially advance order status.
	// Pass orderedQty so markItemsFulfilled uses only DB-authoritative quantities.
	if err := s.markItemsFulfilled(orderID, actorID, items, orderedQty); err != nil {
		return fmt.Errorf("service: IssueItems: mark fulfilled: %w", err)
	}

	for _, item := range items {
		observability.Distribution.Info("item issued", "order_id", orderID, "material_id", item.MaterialID, "qty", item.Qty, "scan_id", scanID, "actor_id", actorID)
	}
	return nil
}

// markItemsFulfilled updates fulfillment_status for each issued item.
// When the issued quantity equals the ordered quantity the item is marked
// "fulfilled"; when it is less, the item is marked "backordered" and a
// backorder record is created for the shortfall.
// The order is advanced from pending_shipment → in_transit on the first issue.
//
// orderedQtys is the DB-authoritative ordered quantity map (materialID → qty)
// built from order_items. Client-supplied item.Qty is never used for
// fulfillment logic — doing so would allow a forged lower qty to fake
// full fulfillment with fewer physical copies.
func (s *DistributionService) markItemsFulfilled(orderID, actorID int64, items []IssueItem, orderedQtys map[int64]int) error {
	for _, item := range items {
		oqty := orderedQtys[item.MaterialID]

		issued := item.IssuedQty
		if issued <= 0 {
			// Default to the full DB-authoritative ordered quantity.
			issued = oqty
		}

		if issued >= oqty {
			// Fully satisfied — mark fulfilled.
			if err := s.orderRepo.MarkOrderItemFulfilled(orderID, item.MaterialID); err != nil {
				return fmt.Errorf("markItemsFulfilled: mark fulfilled material %d: %w", item.MaterialID, err)
			}
		} else {
			// Partial issue — mark backordered and record the shortfall.
			if err := s.orderRepo.MarkOrderItemBackordered(orderID, item.MaterialID); err != nil {
				return fmt.Errorf("markItemsFulfilled: mark backordered material %d: %w", item.MaterialID, err)
			}
			itemID, err := s.orderRepo.GetOrderItemID(orderID, item.MaterialID)
			if err != nil {
				return fmt.Errorf("markItemsFulfilled: get order_item_id material %d: %w", item.MaterialID, err)
			}
			shortfall := oqty - issued // DB-authoritative, not client-provided
			if _, err := s.orderRepo.CreateBackorder(itemID, shortfall); err != nil {
				return fmt.Errorf("markItemsFulfilled: create backorder material %d: %w", item.MaterialID, err)
			}
			observability.Distribution.Warn("partial issue — backorder created",
				"order_id", orderID, "material_id", item.MaterialID,
				"issued", issued, "shortfall", shortfall)
		}
	}

	// Advance order from pending_shipment → in_transit on first issue.
	order, err := s.orderRepo.GetByID(orderID)
	if err != nil {
		return err
	}
	if order.Status == "pending_shipment" {
		if err := s.orderRepo.Transition(orderID, actorID, "in_transit", "items issued by clerk", s.materialRepo); err != nil {
			return fmt.Errorf("markItemsFulfilled: advance to in_transit: %w", err)
		}
	}

	return nil
}

// ---------------------------------------------------------------
// Return
// ---------------------------------------------------------------

// RecordReturn records a "returned" distribution event and releases inventory
// (increments available_qty by qty).
//
// returnRequestID must reference an approved return_request of type "return"
// for orderID. The call is rejected if the request does not exist, is not yet
// approved, or belongs to a different order.
func (s *DistributionService) RecordReturn(orderID, materialID, actorID, returnRequestID int64, scanID string, qty int) error {
	if qty <= 0 {
		return errors.New("service: RecordReturn: qty must be positive")
	}

	// Require an approved return request of the correct type.
	rr, err := s.orderRepo.GetReturnRequestByID(returnRequestID)
	if err != nil {
		return fmt.Errorf("service: RecordReturn: load return request: %w", err)
	}
	if rr.Status != "approved" {
		return fmt.Errorf("service: RecordReturn: return request %d is not approved (status=%s)", returnRequestID, rr.Status)
	}
	if rr.OrderID != orderID {
		return fmt.Errorf("service: RecordReturn: return request %d belongs to order %d, not %d", returnRequestID, rr.OrderID, orderID)
	}
	if rr.Type != "return" {
		return fmt.Errorf("service: RecordReturn: return request %d has wrong type %q (expected 'return')", returnRequestID, rr.Type)
	}

	// Verify order exists.
	if _, err := s.orderRepo.GetByID(orderID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("service: RecordReturn: order %d not found", orderID)
		}
		return fmt.Errorf("service: RecordReturn: load order: %w", err)
	}

	// Validate materialID belongs to this order's line items and that the
	// returned qty does not exceed what was originally ordered.
	orderItems, err := s.orderRepo.GetItemsByOrderID(orderID)
	if err != nil {
		return fmt.Errorf("service: RecordReturn: load order items: %w", err)
	}
	var orderedQty int
	found := false
	for _, oi := range orderItems {
		if oi.MaterialID == materialID {
			orderedQty = oi.Qty
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("service: RecordReturn: material %d is not part of order %d", materialID, orderID)
	}
	if qty > orderedQty {
		return fmt.Errorf("service: RecordReturn: return qty (%d) exceeds ordered qty (%d) for material %d", qty, orderedQty, materialID)
	}

	orderIDPtr := &orderID
	actorIDPtr := &actorID
	scanIDPtr := &scanID

	evt := &models.DistributionEvent{
		OrderID:     orderIDPtr,
		MaterialID:  materialID,
		Qty:         qty,
		EventType:   "returned",
		ScanID:      scanIDPtr,
		ActorID:     actorIDPtr,
		CustodyFrom: stringPtr("student"),
		CustodyTo:   stringPtr("clerk"),
	}
	if _, err := s.distRepo.RecordEvent(evt); err != nil {
		return fmt.Errorf("service: RecordReturn: record event: %w", err)
	}

	// Return copies to available stock. The order is already completed so
	// reserved_qty for these items has already been decremented by the
	// completion transition; we only need to increment available_qty.
	if err := s.materialRepo.ReturnToStock(materialID, qty); err != nil {
		return fmt.Errorf("service: RecordReturn: return to stock: %w", err)
	}

	observability.Distribution.Info("item returned", "order_id", orderID, "material_id", materialID, "qty", qty, "scan_id", scanID, "actor_id", actorID)
	return nil
}

// ---------------------------------------------------------------
// Exchange
// ---------------------------------------------------------------

// RecordExchange swaps an old copy for a new one in a single logical operation:
//  1. Validates the approved exchange request.
//  2. Records a "returned" event for oldMaterialID.
//  3. Verifies the new material has available stock.
//  4. Records an "issued" event for newMaterialID and decrements its
//     available_qty.
//
// returnRequestID must reference an approved return_request of type "exchange"
// for orderID.
func (s *DistributionService) RecordExchange(orderID, oldMaterialID, newMaterialID, actorID, returnRequestID int64, scanID string, qty int) error {
	if qty <= 0 {
		return errors.New("service: RecordExchange: qty must be positive")
	}

	// Require an approved exchange request of the correct type.
	rr, err := s.orderRepo.GetReturnRequestByID(returnRequestID)
	if err != nil {
		return fmt.Errorf("service: RecordExchange: load return request: %w", err)
	}
	if rr.Status != "approved" {
		return fmt.Errorf("service: RecordExchange: return request %d is not approved (status=%s)", returnRequestID, rr.Status)
	}
	if rr.OrderID != orderID {
		return fmt.Errorf("service: RecordExchange: return request %d belongs to order %d, not %d", returnRequestID, rr.OrderID, orderID)
	}
	if rr.Type != "exchange" {
		return fmt.Errorf("service: RecordExchange: return request %d has wrong type %q (expected 'exchange')", returnRequestID, rr.Type)
	}

	// Validate oldMaterialID belongs to this order's line items.
	exchangeItems, err := s.orderRepo.GetItemsByOrderID(orderID)
	if err != nil {
		return fmt.Errorf("service: RecordExchange: load order items: %w", err)
	}
	oldMatFound := false
	for _, oi := range exchangeItems {
		if oi.MaterialID == oldMaterialID {
			oldMatFound = true
			break
		}
	}
	if !oldMatFound {
		return fmt.Errorf("service: RecordExchange: material %d is not part of order %d", oldMaterialID, orderID)
	}

	// Return the old copy (releases its inventory).
	// We pass 0 as returnRequestID to the inner RecordReturn because the
	// exchange request already covers both legs; RecordReturn's own request
	// check is bypassed here via the internal call path.
	if err := s.recordReturnInternal(orderID, oldMaterialID, actorID, scanID, qty); err != nil {
		return fmt.Errorf("service: RecordExchange: return old: %w", err)
	}

	// Check new material stock.
	newMat, err := s.materialRepo.GetByID(newMaterialID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("service: RecordExchange: new material %d not found", newMaterialID)
		}
		return fmt.Errorf("service: RecordExchange: load new material: %w", err)
	}
	if newMat.AvailableQty < qty {
		return fmt.Errorf("service: RecordExchange: insufficient stock for material %q (available %d, requested %d)",
			newMat.Title, newMat.AvailableQty, qty)
	}

	orderIDPtr := &orderID
	actorIDPtr := &actorID
	newScanID := scanID + "_exch"
	scanIDPtr := &newScanID

	evt := &models.DistributionEvent{
		OrderID:     orderIDPtr,
		MaterialID:  newMaterialID,
		Qty:         qty,
		EventType:   "issued",
		ScanID:      scanIDPtr,
		ActorID:     actorIDPtr,
		CustodyFrom: stringPtr("clerk"),
		CustodyTo:   stringPtr("student"),
	}
	if _, err := s.distRepo.RecordEvent(evt); err != nil {
		return fmt.Errorf("service: RecordExchange: record issue event: %w", err)
	}

	// Directly decrement available_qty for the newly issued material. Because
	// the exchange is performed post-fulfillment the new copy goes straight
	// to the student without a reservation phase, so DirectIssue (available
	// only) is correct — Reserve would incorrectly increment reserved_qty.
	if err := s.materialRepo.DirectIssue(newMaterialID, qty); err != nil {
		return fmt.Errorf("service: RecordExchange: direct issue new material: %w", err)
	}

	observability.Distribution.Info("item exchanged", "order_id", orderID, "old_material_id", oldMaterialID, "new_material_id", newMaterialID, "qty", qty, "actor_id", actorID)
	return nil
}


// ---------------------------------------------------------------
// Reissue
// ---------------------------------------------------------------

// ReissueItem handles lost or damaged copy replacement:
//   - Validates that the material has at least one available copy for the
//     replacement (mirrors the availability check in RecordExchange).
//   - Records a "lost" or "damaged" event for oldScanID (marking it out of
//     circulation).
//   - Records a new "issued" event for newScanID.
//   - Decrements available_qty for the replacement copy so the inventory ledger
//     stays consistent with the physical state of the collection.
//
// reason must be "lost" or "damaged".
func (s *DistributionService) ReissueItem(orderID, materialID, actorID int64, oldScanID, newScanID, reason string) error {
	if reason != "lost" && reason != "damaged" {
		return fmt.Errorf("service: ReissueItem: invalid reason %q (must be 'lost' or 'damaged')", reason)
	}
	if oldScanID == "" || newScanID == "" {
		return errors.New("service: ReissueItem: both oldScanID and newScanID are required")
	}

	// Verify order exists.
	if _, err := s.orderRepo.GetByID(orderID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("service: ReissueItem: order %d not found", orderID)
		}
		return fmt.Errorf("service: ReissueItem: load order: %w", err)
	}

	// Verify the material has at least one available copy for the replacement.
	// A reissue takes a fresh copy from the shelf — the lost/damaged copy is
	// already gone, so we must confirm stock before committing the event.
	mat, err := s.materialRepo.GetByID(materialID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("service: ReissueItem: material %d not found", materialID)
		}
		return fmt.Errorf("service: ReissueItem: load material: %w", err)
	}
	if mat.AvailableQty < 1 {
		return fmt.Errorf("service: ReissueItem: no available stock for %q — cannot issue replacement", mat.Title)
	}

	orderIDPtr := &orderID
	actorIDPtr := &actorID

	// Record the loss/damage event for the old scan ID.
	oldEvt := &models.DistributionEvent{
		OrderID:    orderIDPtr,
		MaterialID: materialID,
		Qty:        1,
		EventType:  reason, // "lost" or "damaged"
		ScanID:     &oldScanID,
		ActorID:    actorIDPtr,
	}
	if _, err := s.distRepo.RecordEvent(oldEvt); err != nil {
		return fmt.Errorf("service: ReissueItem: record %s event: %w", reason, err)
	}

	// Issue the replacement copy and record the event.
	newEvt := &models.DistributionEvent{
		OrderID:     orderIDPtr,
		MaterialID:  materialID,
		Qty:         1,
		EventType:   "issued",
		ScanID:      &newScanID,
		ActorID:     actorIDPtr,
		CustodyFrom: stringPtr("clerk"),
		CustodyTo:   stringPtr("student"),
	}
	if _, err := s.distRepo.RecordEvent(newEvt); err != nil {
		return fmt.Errorf("service: ReissueItem: record replacement issue event: %w", err)
	}

	// Decrement available_qty for the replacement copy (mirrors RecordExchange).
	// The lost/damaged copy is already out of circulation, so only the new
	// replacement needs to leave the available pool.
	if err := s.materialRepo.DirectIssue(materialID, 1); err != nil {
		return fmt.Errorf("service: ReissueItem: decrement inventory for replacement: %w", err)
	}

	observability.Distribution.Info("item reissued",
		"order_id", orderID, "material_id", materialID,
		"old_scan_id", oldScanID, "new_scan_id", newScanID,
		"reason", reason, "actor_id", actorID)
	return nil
}

// ---------------------------------------------------------------
// Ledger / chain queries — thin pass-through
// ---------------------------------------------------------------

// GetLedger returns a paginated, filtered view of distribution events.
func (s *DistributionService) GetLedger(filters repository.DistributionFilter, limit, offset int) ([]models.DistributionEvent, error) {
	return s.distRepo.ListEvents(filters, limit, offset)
}

// GetCustodyChain returns the full event history for a physical copy.
func (s *DistributionService) GetCustodyChain(scanID string) ([]models.DistributionEvent, error) {
	if scanID == "" {
		return nil, errors.New("service: GetCustodyChain: scanID is required")
	}
	return s.distRepo.GetCustodyChain(scanID)
}

// GetPendingIssues returns the clerk's pick list.
func (s *DistributionService) GetPendingIssues(limit, offset int) ([]repository.PendingIssue, error) {
	return s.distRepo.GetPendingIssues(limit, offset)
}

// CountBackorders returns the total number of unresolved backorder records.
func (s *DistributionService) CountBackorders() (int, error) {
	return s.distRepo.CountBackorders()
}

// ---------------------------------------------------------------
// helpers
// ---------------------------------------------------------------

// recordReturnInternal records the return event and releases inventory without
// validating a return_request. It is used internally by RecordExchange, which
// has already validated the exchange request before splitting into legs.
func (s *DistributionService) recordReturnInternal(orderID, materialID, actorID int64, scanID string, qty int) error {
	orderIDPtr := &orderID
	actorIDPtr := &actorID
	scanIDPtr := &scanID

	evt := &models.DistributionEvent{
		OrderID:     orderIDPtr,
		MaterialID:  materialID,
		Qty:         qty,
		EventType:   "returned",
		ScanID:      scanIDPtr,
		ActorID:     actorIDPtr,
		CustodyFrom: stringPtr("student"),
		CustodyTo:   stringPtr("clerk"),
	}
	if _, err := s.distRepo.RecordEvent(evt); err != nil {
		return fmt.Errorf("record event: %w", err)
	}
	// Return copies to available stock (post-fulfillment: only available_qty).
	if err := s.materialRepo.ReturnToStock(materialID, qty); err != nil {
		return fmt.Errorf("return to stock: %w", err)
	}
	return nil
}

func stringPtr(s string) *string { return &s }
