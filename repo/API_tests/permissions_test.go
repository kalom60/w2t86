package api_tests

import (
	"fmt"
	"net/http"
	"testing"

	"w2t86/internal/repository"
)

// permissions_test.go verifies role-based access control across the portal.
// Each test focuses on one protected route and checks that:
//   - The required role CAN access it (no 403/401).
//   - A lower-privileged role CANNOT access it (403/401).

// ---------------------------------------------------------------------------
// Moderation queue
// ---------------------------------------------------------------------------

// TestPermission_Moderation_StudentForbidden returns 403 for a student.
func TestPermission_Moderation_StudentForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodGet, "/moderation", "", cookie, "")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for student, got %d", resp.StatusCode)
	}
}

// TestPermission_Moderation_ModeratorAllowed returns non-403 for a moderator.
func TestPermission_Moderation_ModeratorAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "moderator")
	resp := makeRequest(app, http.MethodGet, "/moderation", "", cookie, "")
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		t.Fatalf("moderator should access /moderation, got %d", resp.StatusCode)
	}
}

// TestPermission_Moderation_AdminAllowed returns non-403 for an admin.
func TestPermission_Moderation_AdminAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "admin")
	resp := makeRequest(app, http.MethodGet, "/moderation", "", cookie, "")
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		t.Fatalf("admin should access /moderation, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Distribution
// ---------------------------------------------------------------------------

// TestPermission_Distribution_StudentForbidden returns 403 for a student.
func TestPermission_Distribution_StudentForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodGet, "/distribution", "", cookie, "")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for student, got %d", resp.StatusCode)
	}
}

// TestPermission_Distribution_ClerkAllowed returns non-403 for a clerk.
func TestPermission_Distribution_ClerkAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "clerk")
	resp := makeRequest(app, http.MethodGet, "/distribution", "", cookie, "")
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		t.Fatalf("clerk should access /distribution, got %d", resp.StatusCode)
	}
}

// TestPermission_DistributionLedger_InstructorForbidden returns 403.
func TestPermission_DistributionLedger_InstructorForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "instructor")
	resp := makeRequest(app, http.MethodGet, "/distribution/ledger", "", cookie, "")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for instructor, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Admin panel
// ---------------------------------------------------------------------------

// TestPermission_AdminUsers_ModeratorForbidden returns 403 for a moderator.
func TestPermission_AdminUsers_ModeratorForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "moderator")
	resp := makeRequest(app, http.MethodGet, "/admin/users", "", cookie, "")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for moderator accessing admin users, got %d", resp.StatusCode)
	}
}

// TestPermission_AdminUsers_AdminAllowed returns non-403 for an admin.
func TestPermission_AdminUsers_AdminAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "admin")
	resp := makeRequest(app, http.MethodGet, "/admin/users", "", cookie, "")
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		t.Fatalf("admin should access /admin/users, got %d", resp.StatusCode)
	}
}

// TestPermission_Analytics_ClerkForbidden returns 403 for a clerk.
func TestPermission_Analytics_ClerkForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "clerk")
	resp := makeRequest(app, http.MethodGet, "/analytics/export/orders", "", cookie, "")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for clerk on analytics export, got %d", resp.StatusCode)
	}
}

// TestPermission_Analytics_AdminAllowed returns non-403 for admin.
func TestPermission_Analytics_AdminAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "admin")
	resp := makeRequest(app, http.MethodGet, "/analytics/export/orders", "", cookie, "")
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		t.Fatalf("admin should access analytics export, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Return requests
// ---------------------------------------------------------------------------

// TestPermission_AdminReturns_InstructorAllowed returns non-403 for instructor.
// Instructor is the legacy name for the "manager" role in this system.
func TestPermission_AdminReturns_InstructorAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "instructor")
	resp := makeRequest(app, http.MethodGet, "/admin/returns", "", cookie, "")
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		t.Fatalf("instructor (manager role) should access /admin/returns, got %d", resp.StatusCode)
	}
}

// TestPermission_AdminReturns_ManagerAllowed verifies that a user with the
// explicit "manager" role can access the return-approval queue.
func TestPermission_AdminReturns_ManagerAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "manager")
	resp := makeRequest(app, http.MethodGet, "/admin/returns", "", cookie, "")
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		t.Fatalf("manager role should access /admin/returns, got %d", resp.StatusCode)
	}
}

// TestPermission_AdminReturns_StudentForbidden returns 403 for a student.
func TestPermission_AdminReturns_StudentForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodGet, "/admin/returns", "", cookie, "")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for student on admin returns, got %d", resp.StatusCode)
	}
}

// TestPermission_AdminReturns_ClerkForbidden verifies that a clerk cannot
// access the return-approval queue (manager/instructor role required).
func TestPermission_AdminReturns_ClerkForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "clerk")
	resp := makeRequest(app, http.MethodGet, "/admin/returns", "", cookie, "")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for clerk on admin returns, got %d", resp.StatusCode)
	}
}

// TestPermission_AdminReturns_ModeratorForbidden verifies that a moderator
// cannot access the return-approval queue.
func TestPermission_AdminReturns_ModeratorForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "moderator")
	resp := makeRequest(app, http.MethodGet, "/admin/returns", "", cookie, "")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for moderator on admin returns, got %d", resp.StatusCode)
	}
}

// TestPermission_ApproveReturn_StudentForbidden verifies a student cannot POST
// to the approve-return endpoint (manager role required).
func TestPermission_ApproveReturn_StudentForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodPost, "/admin/returns/1/approve", "", cookie, "application/x-www-form-urlencoded")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for student approving return, got %d", resp.StatusCode)
	}
}

// TestPermission_ApproveReturn_ClerkForbidden verifies a clerk cannot approve
// return requests (only instructor/admin — the manager role — may approve).
func TestPermission_ApproveReturn_ClerkForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "clerk")
	resp := makeRequest(app, http.MethodPost, "/admin/returns/1/approve", "", cookie, "application/x-www-form-urlencoded")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for clerk approving return, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Moderation actions — approve / remove
// ---------------------------------------------------------------------------

// TestPermission_ApproveComment_ModeratorAllowed verifies a moderator can call the approve endpoint.
func TestPermission_ApproveComment_ModeratorAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	// Create a collapsed comment to approve.
	author := createUser(t, db, "student")
	mat := createMaterial(t, db)
	engRepo := repository.NewEngagementRepository(db)
	comment, err := engRepo.CreateComment(mat.ID, author.ID, "test comment", 0)
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}
	// Manually collapse it.
	if _, err := db.Exec(`UPDATE comments SET status='collapsed' WHERE id=?`, comment.ID); err != nil {
		t.Fatalf("collapse comment: %v", err)
	}

	cookie := loginAs(t, app, db, "moderator")
	resp := makeRequest(app, http.MethodPost,
		fmt.Sprintf("/moderation/%d/approve", comment.ID),
		"", cookie, "", htmx())
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		t.Fatalf("moderator should be allowed to approve, got %d", resp.StatusCode)
	}
}

// TestPermission_ApproveComment_StudentForbidden returns 403.
func TestPermission_ApproveComment_StudentForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodPost, "/moderation/1/approve", "", cookie, "", htmx())
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for student approving comment, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Metrics endpoint — admin-only
// ---------------------------------------------------------------------------

// TestPermission_Metrics_AdminAllowed verifies that an admin can access /metrics.
func TestPermission_Metrics_AdminAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "admin")
	resp := makeRequest(app, http.MethodGet, "/metrics", "", cookie, "")
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		t.Fatalf("admin should be allowed to access /metrics, got %d", resp.StatusCode)
	}
}

// TestPermission_Metrics_StudentForbidden verifies that a student cannot access /metrics.
func TestPermission_Metrics_StudentForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodGet, "/metrics", "", cookie, "")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for student on /metrics, got %d", resp.StatusCode)
	}
}

// TestPermission_Metrics_InstructorForbidden verifies that an instructor cannot access /metrics.
func TestPermission_Metrics_InstructorForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "instructor")
	resp := makeRequest(app, http.MethodGet, "/metrics", "", cookie, "")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for instructor on /metrics, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Geospatial analytics map endpoints — admin-only
// ---------------------------------------------------------------------------

// TestPermission_MapData_AdminAllowed verifies an admin can access /analytics/map/data.
func TestPermission_MapData_AdminAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "admin")
	resp := makeRequest(app, http.MethodGet, "/analytics/map/data", "", cookie, "")
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		t.Fatalf("admin should be allowed on /analytics/map/data, got %d", resp.StatusCode)
	}
}

// TestPermission_MapData_StudentForbidden verifies a student cannot access /analytics/map/data.
func TestPermission_MapData_StudentForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodGet, "/analytics/map/data", "", cookie, "")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for student on /analytics/map/data, got %d", resp.StatusCode)
	}
}

// TestPermission_MapCompute_StudentForbidden verifies a student cannot POST /analytics/map/compute.
func TestPermission_MapCompute_StudentForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodPost, "/analytics/map/compute", "", cookie, "")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for student on /analytics/map/compute, got %d", resp.StatusCode)
	}
}

// TestPermission_MapBufferQuery_InstructorForbidden verifies an instructor cannot access
// geospatial buffer endpoint (admin-only).
func TestPermission_MapBufferQuery_InstructorForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "instructor")
	resp := makeRequest(app, http.MethodGet, "/analytics/map/buffer", "", cookie, "")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for instructor on /analytics/map/buffer, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Dashboard stat API — sensitive stats must be admin-only
// ---------------------------------------------------------------------------

// sensitiveStats lists the stat names that expose internal operational metrics
// and must only be served to admin users.
var sensitiveStats = []string{
	"total-orders",
	"active-users",
	"conversion-rate",
	"repeat-purchase",
	"pending-issues",
	"backorders",
	"moderation-queue",
	"course-plans",
	"pending-returns",
}

// TestPermission_Stats_SensitiveStatsForbiddenForStudent verifies that each
// sensitive stat returns 403 when requested by a student.
func TestPermission_Stats_SensitiveStatsForbiddenForStudent(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	for _, stat := range sensitiveStats {
		stat := stat // capture
		t.Run(stat, func(t *testing.T) {
			resp := makeRequest(app, http.MethodGet, fmt.Sprintf("/api/stats/%s", stat), "", cookie, "")
			if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("stat %q: expected 403/401 for student, got %d", stat, resp.StatusCode)
			}
		})
	}
}

// TestPermission_Stats_SensitiveStatsForbiddenForInstructor verifies that each
// sensitive stat returns 403 when requested by an instructor (non-admin).
func TestPermission_Stats_SensitiveStatsForbiddenForInstructor(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "instructor")
	for _, stat := range sensitiveStats {
		stat := stat
		t.Run(stat, func(t *testing.T) {
			resp := makeRequest(app, http.MethodGet, fmt.Sprintf("/api/stats/%s", stat), "", cookie, "")
			if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("stat %q: expected 403/401 for instructor, got %d", stat, resp.StatusCode)
			}
		})
	}
}

// TestPermission_Stats_AdminCanAccessAllStats verifies that an admin can
// retrieve every sensitive stat without a 403/401.
func TestPermission_Stats_AdminCanAccessAllStats(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "admin")
	for _, stat := range sensitiveStats {
		stat := stat
		t.Run(stat, func(t *testing.T) {
			resp := makeRequest(app, http.MethodGet, fmt.Sprintf("/api/stats/%s", stat), "", cookie, "")
			if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
				t.Fatalf("stat %q: admin should have access, got %d", stat, resp.StatusCode)
			}
		})
	}
}
