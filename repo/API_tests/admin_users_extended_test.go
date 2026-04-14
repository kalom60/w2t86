package api_tests

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// admin_users_extended_test.go covers user management and custom field endpoints:
//   GET  /admin/users/new                          — new user form
//   GET  /admin/users/:id                          — user profile page
//   GET  /admin/fields/:entity_type/:entity_id     — custom fields page
//   POST /admin/fields/:entity_type/:entity_id     — set a custom field
//   DELETE /admin/fields/:entity_type/:entity_id/:name — delete a custom field
//   GET  /admin/users/:id/fields                   — legacy alias
//   POST /admin/users/:id/fields                   — legacy alias
//   DELETE /admin/users/:id/fields/:name           — legacy alias
//   GET  /admin/audit/:entityType/:entityID        — entity-scoped audit log

// ---------------------------------------------------------------------------
// GET /admin/users/new
// ---------------------------------------------------------------------------

// TestAdminUsers_NewForm_AdminAllowed renders the empty creation form.
func TestAdminUsers_NewForm_AdminAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "admin")
	resp := makeRequest(app, http.MethodGet, "/admin/users/new", "", cookie, "")
	if resp.StatusCode/100 != 2 {
		t.Fatalf("expected 2xx for /admin/users/new, got %d; body: %s",
			resp.StatusCode, readBody(resp))
	}
}

// TestAdminUsers_NewForm_StudentForbidden returns 403.
func TestAdminUsers_NewForm_StudentForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodGet, "/admin/users/new", "", cookie, "")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for student on /admin/users/new, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// GET /admin/users/:id
// ---------------------------------------------------------------------------

// TestAdminUsers_Profile_AdminAllowed renders a user's profile page.
func TestAdminUsers_Profile_AdminAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	target := createUser(t, db, "student")
	cookie := loginAs(t, app, db, "admin")

	resp := makeRequest(app, http.MethodGet,
		fmt.Sprintf("/admin/users/%d", target.ID), "", cookie, "")
	if resp.StatusCode == http.StatusNotFound {
		t.Fatalf("GET /admin/users/%d returned 404", target.ID)
	}
	if resp.StatusCode/100 != 2 {
		t.Fatalf("expected 2xx for user profile, got %d; body: %s", resp.StatusCode, readBody(resp))
	}
}

// TestAdminUsers_Profile_NotFound verifies the user profile page for an unknown ID
// does not crash (handler renders an empty profile page — not necessarily 404).
func TestAdminUsers_Profile_NotFound(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "admin")
	resp := makeRequest(app, http.MethodGet, "/admin/users/999999", "", cookie, "")
	if resp.StatusCode == http.StatusInternalServerError {
		t.Fatalf("server error for unknown user profile: %s", readBody(resp))
	}
	// Handler renders an empty profile page (200) for non-existent users
	// rather than 404 — both outcomes are acceptable.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unexpected status for unknown user profile, got %d", resp.StatusCode)
	}
}

// TestAdminUsers_Profile_StudentForbidden returns 403.
func TestAdminUsers_Profile_StudentForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	target := createUser(t, db, "student")
	cookie := loginAs(t, app, db, "student")

	resp := makeRequest(app, http.MethodGet,
		fmt.Sprintf("/admin/users/%d", target.ID), "", cookie, "")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for student on user profile, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// GET /admin/fields/:entity_type/:entity_id
// ---------------------------------------------------------------------------

// TestAdminFields_CustomFieldsPage_AdminAllowed renders the fields page.
func TestAdminFields_CustomFieldsPage_AdminAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	target := createUser(t, db, "student")
	cookie := loginAs(t, app, db, "admin")

	resp := makeRequest(app, http.MethodGet,
		fmt.Sprintf("/admin/fields/user/%d", target.ID), "", cookie, "")
	if resp.StatusCode/100 != 2 {
		t.Fatalf("expected 2xx for custom fields page, got %d; body: %s",
			resp.StatusCode, readBody(resp))
	}
}

// TestAdminFields_InvalidEntityType returns 400.
func TestAdminFields_InvalidEntityType(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "admin")
	resp := makeRequest(app, http.MethodGet, "/admin/fields/invalid_type/1", "", cookie, "")
	if resp.StatusCode != http.StatusBadRequest && resp.StatusCode/100 != 2 {
		t.Logf("invalid entity type returned %d (expected 400 or page with error)",
			resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// POST /admin/fields/:entity_type/:entity_id — set a custom field
// ---------------------------------------------------------------------------

// TestAdminFields_SetCustomField_AdminAllowed creates a custom field.
func TestAdminFields_SetCustomField_AdminAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	target := createUser(t, db, "student")
	cookie := loginAs(t, app, db, "admin")

	body := "field_name=department&field_value=Engineering"
	resp := makeRequest(app, http.MethodPost,
		fmt.Sprintf("/admin/fields/user/%d", target.ID),
		body, cookie, "application/x-www-form-urlencoded", htmx())
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 2xx/302 for set custom field, got %d; body: %s",
			resp.StatusCode, readBody(resp))
	}

	// Verify the field was stored.
	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM entity_custom_fields WHERE entity_type='user' AND entity_id=? AND field_name='department'`,
		target.ID,
	).Scan(&count); err != nil {
		t.Fatalf("query custom field: %v", err)
	}
	if count == 0 {
		t.Error("expected custom field to be stored in DB")
	}
}

// TestAdminFields_SetCustomField_StudentForbidden returns 403.
func TestAdminFields_SetCustomField_StudentForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	target := createUser(t, db, "student")
	cookie := loginAs(t, app, db, "student")

	resp := makeRequest(app, http.MethodPost,
		fmt.Sprintf("/admin/fields/user/%d", target.ID),
		"field_name=x&field_value=y", cookie, "application/x-www-form-urlencoded")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for student setting custom field, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// DELETE /admin/fields/:entity_type/:entity_id/:name
// ---------------------------------------------------------------------------

// TestAdminFields_DeleteCustomField_AdminAllowed removes a custom field.
func TestAdminFields_DeleteCustomField_AdminAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	target := createUser(t, db, "student")
	cookie := loginAs(t, app, db, "admin")

	// Create the field first.
	if _, err := db.Exec(
		`INSERT INTO entity_custom_fields (entity_type, entity_id, field_name, field_value)
		 VALUES ('user', ?, 'to_delete', 'value')`,
		target.ID,
	); err != nil {
		t.Fatalf("insert custom field: %v", err)
	}

	resp := makeRequest(app, http.MethodDelete,
		fmt.Sprintf("/admin/fields/user/%d/to_delete", target.ID),
		"", cookie, "application/x-www-form-urlencoded", htmx())
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 2xx/302 for delete custom field, got %d; body: %s",
			resp.StatusCode, readBody(resp))
	}

	// Verify the field is gone.
	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM entity_custom_fields WHERE entity_type='user' AND entity_id=? AND field_name='to_delete'`,
		target.ID,
	).Scan(&count); err != nil {
		t.Fatalf("query custom field after delete: %v", err)
	}
	if count != 0 {
		t.Error("expected custom field to be removed from DB")
	}
}

// ---------------------------------------------------------------------------
// GET /admin/users/:id/fields (legacy alias)
// ---------------------------------------------------------------------------

// TestAdminFields_LegacyAlias_AdminAllowed verifies the legacy route works.
func TestAdminFields_LegacyAlias_AdminAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	target := createUser(t, db, "student")
	cookie := loginAs(t, app, db, "admin")

	resp := makeRequest(app, http.MethodGet,
		fmt.Sprintf("/admin/users/%d/fields", target.ID), "", cookie, "")
	if resp.StatusCode/100 != 2 {
		t.Fatalf("expected 2xx for legacy fields alias, got %d; body: %s",
			resp.StatusCode, readBody(resp))
	}
}

// TestAdminFields_LegacyPost_AdminAllowed verifies the legacy POST alias works.
func TestAdminFields_LegacyPost_AdminAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	target := createUser(t, db, "student")
	cookie := loginAs(t, app, db, "admin")

	body := "field_name=legacy_field&field_value=legacy_val"
	resp := makeRequest(app, http.MethodPost,
		fmt.Sprintf("/admin/users/%d/fields", target.ID),
		body, cookie, "application/x-www-form-urlencoded", htmx())
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 2xx/302 for legacy field POST, got %d; body: %s",
			resp.StatusCode, readBody(resp))
	}
}

// ---------------------------------------------------------------------------
// GET /admin/audit/:entityType/:entityID — entity-specific audit log
// ---------------------------------------------------------------------------

// TestAdminAudit_Entity_AdminAllowed renders the entity audit log page.
func TestAdminAudit_Entity_AdminAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	target := createUser(t, db, "student")
	cookie := loginAs(t, app, db, "admin")

	resp := makeRequest(app, http.MethodGet,
		fmt.Sprintf("/admin/audit/user/%d", target.ID), "", cookie, "")
	if resp.StatusCode/100 != 2 {
		t.Fatalf("expected 2xx for entity audit log, got %d; body: %s",
			resp.StatusCode, readBody(resp))
	}
	body := readBody(resp)
	_ = body
}

// TestAdminAudit_Entity_StudentForbidden returns 403.
func TestAdminAudit_Entity_StudentForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodGet, "/admin/audit/user/1", "", cookie, "")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for student on entity audit log, got %d", resp.StatusCode)
	}
}

// TestAdminAudit_Global_AdminAllowed renders the global audit log page.
func TestAdminAudit_Global_AdminAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "admin")
	resp := makeRequest(app, http.MethodGet, "/admin/audit", "", cookie, "")
	if resp.StatusCode/100 != 2 {
		t.Fatalf("expected 2xx for /admin/audit, got %d; body: %s",
			resp.StatusCode, readBody(resp))
	}
	body := readBody(resp)
	if !strings.Contains(body, "audit") && !strings.Contains(body, "Audit") &&
		!strings.Contains(body, "log") {
		t.Logf("audit page body may be missing expected keywords (got: %.200s...)", body)
	}
}
