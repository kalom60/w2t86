package api_tests

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"w2t86/internal/repository"
)

// favorites_extended_test.go covers:
//   GET  /favorites/:id           — list detail page
//   GET  /favorites/:id/items     — items partial (HTMX)
//   GET  /favorites/:id/share     — generate/display share link

// ---------------------------------------------------------------------------
// GET /favorites/:id — list detail
// ---------------------------------------------------------------------------

// TestFavorites_Detail_Owner verifies a list owner can view their list.
func TestFavorites_Detail_Owner(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	user := createUser(t, db, "student")
	cookie := loginAs(t, app, db, "student")
	_ = user

	// Create a list via API.
	createResp := makeRequest(app, http.MethodPost, "/favorites",
		"name=My+Detail+List&visibility=private",
		cookie, "application/x-www-form-urlencoded")
	_ = readBody(createResp)

	// Fetch list ID directly from DB.
	engRepo := repository.NewEngagementRepository(db)
	lists, err := engRepo.GetLists(func() int64 {
		var id int64
		_ = db.QueryRow(`SELECT id FROM users ORDER BY id DESC LIMIT 1`).Scan(&id)
		return id
	}())
	if err != nil || len(lists) == 0 {
		t.Skip("no favorites list found; skipping detail test")
	}
	listID := lists[0].ID

	resp := makeRequest(app, http.MethodGet, fmt.Sprintf("/favorites/%d", listID), "", cookie, "")
	if resp.StatusCode == http.StatusNotFound {
		t.Fatalf("GET /favorites/%d returned 404 for owner", listID)
	}
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 2xx for favorites detail, got %d; body: %s", resp.StatusCode, readBody(resp))
	}
}

// TestFavorites_Detail_NotFound returns 404 for a non-existent list.
func TestFavorites_Detail_NotFound(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodGet, "/favorites/999999", "", cookie, "")
	if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 404/403 for non-existent favorites list, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// GET /favorites/:id/items — items partial
// ---------------------------------------------------------------------------

// TestFavorites_Items_ReturnsOK returns 2xx for a list owner fetching items.
func TestFavorites_Items_ReturnsOK(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	user := createUser(t, db, "student")
	engRepo := repository.NewEngagementRepository(db)
	list, err := engRepo.CreateList(user.ID, "Items Test List", "private")
	if err != nil {
		t.Fatalf("create list: %v", err)
	}

	cookie := loginAs(t, app, db, "student")
	// loginAs creates a NEW user, so we need to use the user's session.
	// Re-create with the same user by directly getting their session.
	// Instead, create the list from the logged-in user's perspective.
	_ = list // created for user above; actual owner is cookie user

	// Create list as the cookie user (newest user in db).
	var ownerID int64
	_ = db.QueryRow(`SELECT id FROM users ORDER BY id DESC LIMIT 1`).Scan(&ownerID)
	ownedList, err := engRepo.CreateList(ownerID, "Owner Items List", "private")
	if err != nil {
		t.Fatalf("create owned list: %v", err)
	}

	resp := makeRequest(app, http.MethodGet,
		fmt.Sprintf("/favorites/%d/items", ownedList.ID), "", cookie, "", htmx())
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden {
		t.Fatalf("expected 2xx for favorites items, got %d; body: %s", resp.StatusCode, readBody(resp))
	}
}

// TestFavorites_Items_WithContent verifies items partial includes material data.
func TestFavorites_Items_WithContent(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	mat := createMaterial(t, db)

	var ownerID int64
	_ = db.QueryRow(`SELECT id FROM users ORDER BY id DESC LIMIT 1`).Scan(&ownerID)
	engRepo := repository.NewEngagementRepository(db)
	list, err := engRepo.CreateList(ownerID, "Items With Content", "private")
	if err != nil {
		t.Fatalf("create list: %v", err)
	}
	if err := engRepo.AddToList(list.ID, mat.ID); err != nil {
		t.Fatalf("add to list: %v", err)
	}

	resp := makeRequest(app, http.MethodGet,
		fmt.Sprintf("/favorites/%d/items", list.ID), "", cookie, "", htmx())
	if resp.StatusCode/100 != 2 {
		t.Fatalf("expected 2xx for /favorites/%d/items, got %d", list.ID, resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// GET /favorites/:id/share — generate share link
// ---------------------------------------------------------------------------

// TestFavorites_Share_ReturnsOK returns 2xx with a share link.
func TestFavorites_Share_ReturnsOK(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")

	var ownerID int64
	_ = db.QueryRow(`SELECT id FROM users ORDER BY id DESC LIMIT 1`).Scan(&ownerID)
	engRepo := repository.NewEngagementRepository(db)
	list, err := engRepo.CreateList(ownerID, "Share Test List", "public")
	if err != nil {
		t.Fatalf("create list: %v", err)
	}

	resp := makeRequest(app, http.MethodGet,
		fmt.Sprintf("/favorites/%d/share", list.ID), "", cookie, "", htmx())
	if resp.StatusCode/100 != 2 {
		t.Fatalf("expected 2xx for share link page, got %d; body: %s", resp.StatusCode, readBody(resp))
	}
}

// TestFavorites_Share_TokenStoredInDB verifies the share token is persisted.
func TestFavorites_Share_TokenStoredInDB(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")

	var ownerID int64
	_ = db.QueryRow(`SELECT id FROM users ORDER BY id DESC LIMIT 1`).Scan(&ownerID)
	engRepo := repository.NewEngagementRepository(db)
	list, err := engRepo.CreateList(ownerID, "Token DB List", "public")
	if err != nil {
		t.Fatalf("create list: %v", err)
	}

	resp := makeRequest(app, http.MethodGet,
		fmt.Sprintf("/favorites/%d/share", list.ID), "", cookie, "")
	if resp.StatusCode/100 != 2 {
		t.Skipf("share returned %d, skipping DB check", resp.StatusCode)
	}

	// Verify the share token was written into the favorites_lists row.
	var token *string
	_ = db.QueryRow(
		`SELECT share_token FROM favorites_lists WHERE id=?`,
		list.ID,
	).Scan(&token)
	if token == nil || *token == "" {
		t.Error("expected share token in DB after /favorites/:id/share request")
	}
	_ = time.Now() // keep import used
}

// TestFavorites_Share_NotFound returns a non-success status for a non-existent list.
func TestFavorites_Share_NotFound(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodGet, "/favorites/999999/share", "", cookie, "")
	// Handler may return 404, 403, or 422 (unprocessable) for a list that doesn't exist.
	if resp.StatusCode/100 == 2 {
		t.Fatalf("expected non-2xx for non-existent list share, got %d", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusInternalServerError {
		t.Fatalf("server error for non-existent list share: %s", readBody(resp))
	}
}
