package integration_test

import (
	"database/sql"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// createTestCourse inserts a course owned by the given instructorID and returns its ID.
func createTestCourse(t *testing.T, db *sql.DB, instructorID int64) int64 {
	t.Helper()
	name := fmt.Sprintf("Test Course %d", time.Now().UnixNano())
	var id int64
	if err := db.QueryRow(
		`INSERT INTO courses (instructor_id, name) VALUES (?, ?) RETURNING id`,
		instructorID, name,
	).Scan(&id); err != nil {
		t.Fatalf("createTestCourse: %v", err)
	}
	return id
}

// createTestPlanItem inserts a pending course_plans row and returns its ID.
func createTestPlanItem(t *testing.T, db *sql.DB, courseID, materialID int64) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(
		`INSERT INTO course_plans (course_id, material_id, requested_qty, status)
		 VALUES (?, ?, 1, 'pending') RETURNING id`,
		courseID, materialID,
	).Scan(&id); err != nil {
		t.Fatalf("createTestPlanItem: %v", err)
	}
	return id
}

// ---------------------------------------------------------------------------
// AddPlanItem ownership
// ---------------------------------------------------------------------------

// TestAddPlanItem_OwnerCanAdd verifies that the owning instructor can add a
// plan item to their own course.
func TestAddPlanItem_OwnerCanAdd(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	owner := createTestUser(t, db, "instructor")
	courseID := createTestCourse(t, db, owner.ID)
	mat := createTestMaterial(t, db)

	ownerCookie := loginAsUser(t, app, db, owner)

	body := fmt.Sprintf("material_id=%d&requested_qty=2", mat.ID)
	resp := makeRequest(app, http.MethodPost, fmt.Sprintf("/courses/%d/plan", courseID),
		body, ownerCookie, "application/x-www-form-urlencoded", htmxHeaders())

	if resp.StatusCode >= 400 {
		t.Fatalf("expected success for owner adding plan item, got %d; body: %s",
			resp.StatusCode, readBody(resp))
	}
}

// TestAddPlanItem_NonOwnerGets4xx verifies that an instructor who does not own
// the course cannot add a plan item.
func TestAddPlanItem_NonOwnerGets4xx(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	owner := createTestUser(t, db, "instructor")
	courseID := createTestCourse(t, db, owner.ID)
	mat := createTestMaterial(t, db)

	otherCookie := loginAs(t, app, db, "instructor")

	body := fmt.Sprintf("material_id=%d&requested_qty=1", mat.ID)
	resp := makeRequest(app, http.MethodPost, fmt.Sprintf("/courses/%d/plan", courseID),
		body, otherCookie, "application/x-www-form-urlencoded", htmxHeaders())

	if resp.StatusCode < 400 {
		t.Errorf("expected 4xx for non-owner adding plan item, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// ApprovePlanItem ownership
// ---------------------------------------------------------------------------

// TestApprovePlanItem_OwnerCanApprove verifies that the owning instructor can
// approve a plan item on their own course.
func TestApprovePlanItem_OwnerCanApprove(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	owner := createTestUser(t, db, "instructor")
	courseID := createTestCourse(t, db, owner.ID)
	mat := createTestMaterial(t, db)
	planID := createTestPlanItem(t, db, courseID, mat.ID)

	ownerCookie := loginAsUser(t, app, db, owner)

	body := "approved_qty=1"
	resp := makeRequest(app, http.MethodPost,
		fmt.Sprintf("/courses/%d/plan/%d/approve", courseID, planID),
		body, ownerCookie, "application/x-www-form-urlencoded", htmxHeaders())

	if resp.StatusCode >= 400 {
		t.Fatalf("expected success for owner approving plan item, got %d; body: %s",
			resp.StatusCode, readBody(resp))
	}
}

// TestApprovePlanItem_NonOwnerGets4xx verifies that a non-owner instructor
// cannot approve a plan item on a course they don't own.
func TestApprovePlanItem_NonOwnerGets4xx(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	owner := createTestUser(t, db, "instructor")
	courseID := createTestCourse(t, db, owner.ID)
	mat := createTestMaterial(t, db)
	planID := createTestPlanItem(t, db, courseID, mat.ID)

	otherCookie := loginAs(t, app, db, "instructor")

	body := "approved_qty=1"
	resp := makeRequest(app, http.MethodPost,
		fmt.Sprintf("/courses/%d/plan/%d/approve", courseID, planID),
		body, otherCookie, "application/x-www-form-urlencoded", htmxHeaders())

	if resp.StatusCode < 400 {
		t.Errorf("expected 4xx for non-owner approving plan item, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// AddSection ownership
// ---------------------------------------------------------------------------

// TestAddSection_OwnerCanAdd verifies that the owning instructor can add a
// section to their course.
func TestAddSection_OwnerCanAdd(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	owner := createTestUser(t, db, "instructor")
	courseID := createTestCourse(t, db, owner.ID)
	ownerCookie := loginAsUser(t, app, db, owner)

	body := "name=Period+1&period=08:00&room=101"
	resp := makeRequest(app, http.MethodPost,
		fmt.Sprintf("/courses/%d/sections", courseID),
		body, ownerCookie, "application/x-www-form-urlencoded", htmxHeaders())

	if resp.StatusCode >= 400 {
		t.Fatalf("expected success for owner adding section, got %d; body: %s",
			resp.StatusCode, readBody(resp))
	}
}

// TestAddSection_NonOwnerGets4xx verifies that a non-owner instructor cannot
// add a section to someone else's course.
func TestAddSection_NonOwnerGets4xx(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	owner := createTestUser(t, db, "instructor")
	courseID := createTestCourse(t, db, owner.ID)

	otherCookie := loginAs(t, app, db, "instructor")

	body := "name=Period+2"
	resp := makeRequest(app, http.MethodPost,
		fmt.Sprintf("/courses/%d/sections", courseID),
		body, otherCookie, "application/x-www-form-urlencoded", htmxHeaders())

	if resp.StatusCode < 400 {
		t.Errorf("expected 4xx for non-owner adding section, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Role enforcement
// ---------------------------------------------------------------------------

// TestCourses_StudentGets403 verifies that a student is denied access to the
// courses list endpoint.
func TestCourses_StudentGets403(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	studentCookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodGet, "/courses", "", studentCookie, "")

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 for student on /courses, got %d", resp.StatusCode)
	}
}
