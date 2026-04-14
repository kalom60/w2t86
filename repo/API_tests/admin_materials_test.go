package api_tests

import (
	"fmt"
	"net/http"
	"testing"
)

// admin_materials_test.go covers material management endpoints:
//   GET  /admin/materials/new     — new material form
//   GET  /admin/materials/:id/edit — edit material form
//   PUT  /admin/materials/:id     — update material
//   DELETE /admin/materials/:id  — delete material

// ---------------------------------------------------------------------------
// GET /admin/materials/new
// ---------------------------------------------------------------------------

// TestAdminMaterials_NewForm_AdminAllowed renders the empty creation form.
func TestAdminMaterials_NewForm_AdminAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "admin")
	resp := makeRequest(app, http.MethodGet, "/admin/materials/new", "", cookie, "")
	if resp.StatusCode/100 != 2 {
		t.Fatalf("expected 2xx for /admin/materials/new, got %d; body: %s",
			resp.StatusCode, readBody(resp))
	}
}

// TestAdminMaterials_NewForm_StudentForbidden returns 403.
func TestAdminMaterials_NewForm_StudentForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodGet, "/admin/materials/new", "", cookie, "")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for student on new material form, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// GET /admin/materials/:id/edit
// ---------------------------------------------------------------------------

// TestAdminMaterials_EditForm_AdminAllowed renders the form for an existing material.
func TestAdminMaterials_EditForm_AdminAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	mat := createMaterial(t, db)
	cookie := loginAs(t, app, db, "admin")

	resp := makeRequest(app, http.MethodGet,
		fmt.Sprintf("/admin/materials/%d/edit", mat.ID), "", cookie, "")
	if resp.StatusCode == http.StatusNotFound {
		t.Fatalf("GET /admin/materials/%d/edit returned 404", mat.ID)
	}
	if resp.StatusCode/100 != 2 {
		t.Fatalf("expected 2xx for edit material form, got %d; body: %s",
			resp.StatusCode, readBody(resp))
	}
}

// TestAdminMaterials_EditForm_NotFound returns 404 for unknown material.
func TestAdminMaterials_EditForm_NotFound(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "admin")
	resp := makeRequest(app, http.MethodGet, "/admin/materials/999999/edit", "", cookie, "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown material edit, got %d", resp.StatusCode)
	}
}

// TestAdminMaterials_EditForm_StudentForbidden returns 403.
func TestAdminMaterials_EditForm_StudentForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodGet, "/admin/materials/1/edit", "", cookie, "")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for student on edit material form, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// PUT /admin/materials/:id
// ---------------------------------------------------------------------------

// TestAdminMaterials_Update_AdminAllowed updates an existing material title.
func TestAdminMaterials_Update_AdminAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	mat := createMaterial(t, db)
	cookie := loginAs(t, app, db, "admin")

	body := "title=Updated+Material+Title&status=active"
	resp := makeRequest(app, http.MethodPut,
		fmt.Sprintf("/admin/materials/%d", mat.ID),
		body, cookie, "application/x-www-form-urlencoded", htmx())
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 2xx/302 for material update, got %d; body: %s",
			resp.StatusCode, readBody(resp))
	}

	// Verify the title was updated.
	var title string
	if err := db.QueryRow(`SELECT title FROM materials WHERE id=?`, mat.ID).Scan(&title); err != nil {
		t.Fatalf("query updated material: %v", err)
	}
	if title != "Updated Material Title" {
		t.Errorf("expected updated title, got %q", title)
	}
}

// TestAdminMaterials_Update_NotFound verifies that updating an unknown material
// does not crash (no 500) and is not rejected as an unknown route (no 405).
func TestAdminMaterials_Update_NotFound(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "admin")
	resp := makeRequest(app, http.MethodPut, "/admin/materials/999999",
		"title=Phantom", cookie, "application/x-www-form-urlencoded")
	if resp.StatusCode == http.StatusMethodNotAllowed {
		t.Fatalf("PUT /admin/materials/:id returned 405 — route not registered")
	}
	if resp.StatusCode == http.StatusInternalServerError {
		t.Fatalf("server error updating non-existent material: %s", readBody(resp))
	}
}

// TestAdminMaterials_Update_StudentForbidden returns 403.
func TestAdminMaterials_Update_StudentForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodPut, "/admin/materials/1",
		"title=Hacked", cookie, "application/x-www-form-urlencoded")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for student on material update, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// DELETE /admin/materials/:id
// ---------------------------------------------------------------------------

// TestAdminMaterials_Delete_AdminAllowed soft-deletes an existing material.
func TestAdminMaterials_Delete_AdminAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	mat := createMaterial(t, db)
	cookie := loginAs(t, app, db, "admin")

	resp := makeRequest(app, http.MethodDelete,
		fmt.Sprintf("/admin/materials/%d", mat.ID),
		"", cookie, "application/x-www-form-urlencoded", htmx())
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 2xx/302 for material delete, got %d; body: %s",
			resp.StatusCode, readBody(resp))
	}
}

// TestAdminMaterials_Delete_NotFound verifies that deleting an unknown material
// does not crash (no 500) and is not rejected as an unknown route (no 405).
func TestAdminMaterials_Delete_NotFound(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "admin")
	resp := makeRequest(app, http.MethodDelete, "/admin/materials/999999",
		"", cookie, "application/x-www-form-urlencoded")
	if resp.StatusCode == http.StatusMethodNotAllowed {
		t.Fatalf("DELETE /admin/materials/:id returned 405 — route not registered")
	}
	if resp.StatusCode == http.StatusInternalServerError {
		t.Fatalf("server error deleting non-existent material: %s", readBody(resp))
	}
}

// TestAdminMaterials_Delete_StudentForbidden returns 403.
func TestAdminMaterials_Delete_StudentForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodDelete, "/admin/materials/1",
		"", cookie, "")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for student on material delete, got %d", resp.StatusCode)
	}
}
