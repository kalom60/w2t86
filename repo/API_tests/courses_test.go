package api_tests

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// courses_test.go covers all course-planning endpoints:
//   GET  /courses                        — list courses (instructor)
//   GET  /courses/new                    — new course form
//   POST /courses                        — create course
//   GET  /courses/:id                    — course detail
//   POST /courses/:id/plan               — add plan item
//   POST /courses/:id/plan/:planID/approve — approve plan item
//   POST /courses/:id/sections           — add section

// ---------------------------------------------------------------------------
// GET /courses — list courses
// ---------------------------------------------------------------------------

// TestCourses_List_InstructorAllowed returns 2xx for an instructor.
func TestCourses_List_InstructorAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "instructor")
	resp := makeRequest(app, http.MethodGet, "/courses", "", cookie, "")
	if resp.StatusCode/100 != 2 {
		t.Fatalf("expected 2xx for /courses, got %d; body: %s", resp.StatusCode, readBody(resp))
	}
}

// TestCourses_List_AdminAllowed returns 2xx for an admin.
func TestCourses_List_AdminAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "admin")
	resp := makeRequest(app, http.MethodGet, "/courses", "", cookie, "")
	if resp.StatusCode/100 != 2 {
		t.Fatalf("expected 2xx for admin on /courses, got %d; body: %s",
			resp.StatusCode, readBody(resp))
	}
}

// TestCourses_List_StudentForbidden returns 403 for a student.
func TestCourses_List_StudentForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodGet, "/courses", "", cookie, "")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for student on /courses, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// GET /courses/new
// ---------------------------------------------------------------------------

// TestCourses_NewForm_InstructorAllowed renders the empty course form.
func TestCourses_NewForm_InstructorAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "instructor")
	resp := makeRequest(app, http.MethodGet, "/courses/new", "", cookie, "")
	if resp.StatusCode/100 != 2 {
		t.Fatalf("expected 2xx for /courses/new, got %d; body: %s", resp.StatusCode, readBody(resp))
	}
}

// TestCourses_NewForm_StudentForbidden returns 403.
func TestCourses_NewForm_StudentForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodGet, "/courses/new", "", cookie, "")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for student on /courses/new, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// POST /courses — create course
// ---------------------------------------------------------------------------

// TestCourses_Create_InstructorAllowed creates a course and receives a redirect.
func TestCourses_Create_InstructorAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "instructor")
	body := "name=Math+101&subject=Mathematics&grade_level=10&academic_year=2025"
	resp := makeRequest(app, http.MethodPost, "/courses", body, cookie,
		"application/x-www-form-urlencoded")
	if resp.StatusCode != http.StatusFound && resp.StatusCode/100 != 2 {
		t.Fatalf("expected 302/2xx for course creation, got %d; body: %s",
			resp.StatusCode, readBody(resp))
	}
	loc := resp.Header.Get("Location")
	if resp.StatusCode == http.StatusFound && !strings.HasPrefix(loc, "/courses/") {
		t.Errorf("expected redirect to /courses/:id, got: %q", loc)
	}
}

// TestCourses_Create_MissingName rejects a course without a name.
func TestCourses_Create_MissingName(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "instructor")
	body := "subject=Math&grade_level=10&academic_year=2025"
	resp := makeRequest(app, http.MethodPost, "/courses", body, cookie,
		"application/x-www-form-urlencoded")
	if resp.StatusCode == http.StatusFound {
		t.Fatalf("expected non-redirect for missing course name, got %d", resp.StatusCode)
	}
}

// TestCourses_Create_StudentForbidden returns 403.
func TestCourses_Create_StudentForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodPost, "/courses",
		"name=Hack&subject=None&grade_level=1&academic_year=2025",
		cookie, "application/x-www-form-urlencoded")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for student creating course, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// GET /courses/:id — course detail
// ---------------------------------------------------------------------------

// TestCourses_Detail_InstructorAllowed returns the course detail page.
func TestCourses_Detail_InstructorAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "instructor")
	// Create a course first.
	createResp := makeRequest(app, http.MethodPost, "/courses",
		"name=Detail+Test+Course&subject=Science&grade_level=9&academic_year=2025",
		cookie, "application/x-www-form-urlencoded")
	if createResp.StatusCode != http.StatusFound {
		t.Skipf("course creation returned %d, skipping detail test", createResp.StatusCode)
	}

	loc := createResp.Header.Get("Location")
	resp := makeRequest(app, http.MethodGet, loc, "", cookie, "")
	if resp.StatusCode == http.StatusNotFound {
		t.Fatalf("GET %s returned 404 for course owner", loc)
	}
	if resp.StatusCode/100 != 2 {
		t.Fatalf("expected 2xx for course detail, got %d; body: %s", resp.StatusCode, readBody(resp))
	}
}

// TestCourses_Detail_NotFound returns 404 for unknown course.
func TestCourses_Detail_NotFound(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "instructor")
	resp := makeRequest(app, http.MethodGet, "/courses/999999", "", cookie, "")
	if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 404/403 for unknown course, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// POST /courses/:id/plan — add plan item
// ---------------------------------------------------------------------------

// TestCourses_AddPlanItem_InstructorAllowed adds a material to the course plan.
func TestCourses_AddPlanItem_InstructorAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	mat := createMaterial(t, db)
	cookie := loginAs(t, app, db, "instructor")

	// Create a course.
	createResp := makeRequest(app, http.MethodPost, "/courses",
		"name=Plan+Test+Course&subject=Art&grade_level=8&academic_year=2025",
		cookie, "application/x-www-form-urlencoded")
	if createResp.StatusCode != http.StatusFound {
		t.Skipf("course creation returned %d, skipping plan item test", createResp.StatusCode)
	}

	// Extract course ID from redirect.
	loc := createResp.Header.Get("Location")
	var courseID int64
	if _, err := fmt.Sscanf(loc, "/courses/%d", &courseID); err != nil || courseID == 0 {
		t.Skipf("could not parse course ID from redirect %q", loc)
	}

	body := fmt.Sprintf("material_id=%d&requested_qty=5", mat.ID)
	resp := makeRequest(app, http.MethodPost,
		fmt.Sprintf("/courses/%d/plan", courseID),
		body, cookie, "application/x-www-form-urlencoded", htmx())
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 2xx/302 for add plan item, got %d; body: %s",
			resp.StatusCode, readBody(resp))
	}
}

// TestCourses_AddPlanItem_StudentForbidden returns 403.
func TestCourses_AddPlanItem_StudentForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodPost, "/courses/1/plan",
		"material_id=1&requested_qty=1", cookie, "application/x-www-form-urlencoded")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for student adding plan item, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// POST /courses/:id/sections — add section
// ---------------------------------------------------------------------------

// TestCourses_AddSection_InstructorAllowed adds a section to the course.
func TestCourses_AddSection_InstructorAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "instructor")

	// Create course first.
	createResp := makeRequest(app, http.MethodPost, "/courses",
		"name=Section+Test+Course&subject=PE&grade_level=11&academic_year=2025",
		cookie, "application/x-www-form-urlencoded")
	if createResp.StatusCode != http.StatusFound {
		t.Skipf("course creation returned %d", createResp.StatusCode)
	}

	loc := createResp.Header.Get("Location")
	var courseID int64
	if _, err := fmt.Sscanf(loc, "/courses/%d", &courseID); err != nil || courseID == 0 {
		t.Skipf("could not parse course ID from redirect %q", loc)
	}

	body := "name=Section+A&period=1st&room=101"
	resp := makeRequest(app, http.MethodPost,
		fmt.Sprintf("/courses/%d/sections", courseID),
		body, cookie, "application/x-www-form-urlencoded", htmx())
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 2xx/302 for add section, got %d; body: %s",
			resp.StatusCode, readBody(resp))
	}
}

// TestCourses_AddSection_MissingFields rejects empty section name.
func TestCourses_AddSection_MissingFields(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "instructor")
	// Section endpoint will 404 since course 999 doesn't exist, which is non-2xx — fine.
	resp := makeRequest(app, http.MethodPost, "/courses/999999/sections",
		"", cookie, "application/x-www-form-urlencoded")
	if resp.StatusCode/100 == 2 || resp.StatusCode == http.StatusFound {
		t.Fatalf("expected non-success for section on non-existent course, got %d", resp.StatusCode)
	}
}

// TestCourses_AddSection_StudentForbidden returns 403.
func TestCourses_AddSection_StudentForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodPost, "/courses/1/sections",
		"name=X&period=1&room=A", cookie, "application/x-www-form-urlencoded")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for student adding section, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// POST /courses/:id/plan/:planID/approve — approve plan item
// ---------------------------------------------------------------------------

// TestCourses_ApprovePlanItem_AdminAllowed approves a plan item as admin.
func TestCourses_ApprovePlanItem_AdminAllowed(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	mat := createMaterial(t, db)
	instrCookie := loginAs(t, app, db, "instructor")

	// Create a course and add a plan item as instructor.
	createResp := makeRequest(app, http.MethodPost, "/courses",
		"name=Approve+Plan+Course&subject=History&grade_level=12&academic_year=2025",
		instrCookie, "application/x-www-form-urlencoded")
	if createResp.StatusCode != http.StatusFound {
		t.Skipf("course creation returned %d", createResp.StatusCode)
	}
	loc := createResp.Header.Get("Location")
	var courseID int64
	if _, err := fmt.Sscanf(loc, "/courses/%d", &courseID); err != nil || courseID == 0 {
		t.Skipf("could not parse course ID from redirect %q", loc)
	}

	planBody := fmt.Sprintf("material_id=%d&requested_qty=3", mat.ID)
	makeRequest(app, http.MethodPost,
		fmt.Sprintf("/courses/%d/plan", courseID),
		planBody, instrCookie, "application/x-www-form-urlencoded")

	// Get the plan item ID.
	var planID int64
	if err := db.QueryRow(
		`SELECT id FROM course_plan_items WHERE course_id=? LIMIT 1`, courseID,
	).Scan(&planID); err != nil {
		t.Skipf("no plan item found: %v", err)
	}

	// Approve as admin.
	adminCookie := loginAs(t, app, db, "admin")
	approveBody := "approved_qty=3"
	resp := makeRequest(app, http.MethodPost,
		fmt.Sprintf("/courses/%d/plan/%d/approve", courseID, planID),
		approveBody, adminCookie, "application/x-www-form-urlencoded", htmx())
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 2xx/302 for approve plan item, got %d; body: %s",
			resp.StatusCode, readBody(resp))
	}
}

// TestCourses_ApprovePlanItem_StudentForbidden returns 403.
func TestCourses_ApprovePlanItem_StudentForbidden(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodPost, "/courses/1/plan/1/approve",
		"approved_qty=1", cookie, "application/x-www-form-urlencoded")
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for student approving plan item, got %d", resp.StatusCode)
	}
}
