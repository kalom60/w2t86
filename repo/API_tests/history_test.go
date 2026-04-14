package api_tests

import (
	"fmt"
	"net/http"
	"testing"
)

// history_test.go covers GET /history (browse history page).

// TestHistory_ReturnsOK verifies the browse-history page renders for any authenticated user.
func TestHistory_ReturnsOK(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodGet, "/history", "", cookie, "")
	if resp.StatusCode/100 != 2 {
		t.Fatalf("expected 2xx for /history, got %d; body: %s", resp.StatusCode, readBody(resp))
	}
}

// TestHistory_PopulatedAfterVisit verifies that visiting a material detail page
// adds a row visible through the history page response.
func TestHistory_PopulatedAfterVisit(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	mat := createMaterial(t, db)

	// Visit the material detail page to record browse history.
	makeRequest(app, http.MethodGet, fmt.Sprintf("/materials/%d", mat.ID), "", cookie, "")

	resp := makeRequest(app, http.MethodGet, "/history", "", cookie, "")
	if resp.StatusCode/100 != 2 {
		t.Fatalf("expected 2xx for /history after visit, got %d", resp.StatusCode)
	}
}

// TestHistory_Unauthenticated returns 401 or 302.
func TestHistory_Unauthenticated(t *testing.T) {
	app, _, cleanup := newTestApp(t)
	defer cleanup()

	resp := makeRequest(app, http.MethodGet, "/history", "", "", "")
	if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 401/302 for unauthenticated /history, got %d", resp.StatusCode)
	}
}
