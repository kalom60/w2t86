package integration_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"w2t86/internal/repository"
)

// TestModerationQueue_Empty verifies GET /moderation for a moderator returns 200.
func TestModerationQueue_Empty(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	modCookie := loginAs(t, app, db, "moderator")

	resp := makeRequest(app, http.MethodGet, "/moderation", "", modCookie, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for moderator on /moderation, got %d; body: %s",
			resp.StatusCode, readBody(resp))
	}
}

// TestModerationQueue_RequiresModerator verifies that a student cannot access
// GET /moderation and receives 403 Forbidden.
func TestModerationQueue_RequiresModerator(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	studentCookie := loginAs(t, app, db, "student")

	resp := makeRequest(app, http.MethodGet, "/moderation", "", studentCookie, "")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for student on /moderation, got %d", resp.StatusCode)
	}
}

// TestApproveComment verifies POST /moderation/:id/approve transitions a
// collapsed comment to active status in the DB.
func TestApproveComment(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	// Create a comment author.
	author := createTestUser(t, db, "student")
	mat := createTestMaterial(t, db)

	// Create comment and collapse it (set status directly).
	engRepo := repository.NewEngagementRepository(db)
	comment, err := engRepo.CreateComment(mat.ID, author.ID, "This needs review", 0)
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}

	// Set it to collapsed directly in DB.
	if _, err := db.Exec(`UPDATE comments SET status = 'collapsed' WHERE id = ?`, comment.ID); err != nil {
		t.Fatalf("set collapsed: %v", err)
	}

	modCookie := loginAs(t, app, db, "moderator")

	resp := makeRequest(app, http.MethodPost,
		fmt.Sprintf("/moderation/%d/approve", comment.ID),
		"", modCookie, "application/x-www-form-urlencoded", htmxHeaders())

	// On HTMX request, the handler returns 200 with empty body.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 200 or 302 on approve, got %d; body: %s",
			resp.StatusCode, readBody(resp))
	}

	// Verify DB: approved comment returns to 'visible' (not 'active') so the
	// auto-collapse cycle continues to work if the comment is re-reported.
	var status string
	if err := db.QueryRow(`SELECT status FROM comments WHERE id = ?`, comment.ID).Scan(&status); err != nil {
		t.Fatalf("query comment: %v", err)
	}
	if status != "visible" {
		t.Errorf("expected comment status 'visible' after approve, got %q", status)
	}
}

// TestRemoveComment verifies POST /moderation/:id/remove transitions a collapsed
// comment to removed status in the DB.
func TestRemoveComment(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	author := createTestUser(t, db, "student")
	mat := createTestMaterial(t, db)

	engRepo := repository.NewEngagementRepository(db)
	comment, err := engRepo.CreateComment(mat.ID, author.ID, "Spam comment", 0)
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}
	if _, err := db.Exec(`UPDATE comments SET status = 'collapsed' WHERE id = ?`, comment.ID); err != nil {
		t.Fatalf("set collapsed: %v", err)
	}

	modCookie := loginAs(t, app, db, "moderator")

	resp := makeRequest(app, http.MethodPost,
		fmt.Sprintf("/moderation/%d/remove", comment.ID),
		"", modCookie, "application/x-www-form-urlencoded", htmxHeaders())

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 200 or 302 on remove, got %d; body: %s",
			resp.StatusCode, readBody(resp))
	}

	var status string
	if err := db.QueryRow(`SELECT status FROM comments WHERE id = ?`, comment.ID).Scan(&status); err != nil {
		t.Fatalf("query comment: %v", err)
	}
	if status != "removed" {
		t.Errorf("expected comment status 'removed' after remove, got %q", status)
	}
}

// TestCollapsedComment_HiddenFromNonModerator verifies that a comment in
// 'collapsed' state is not returned to regular users (students/instructors).
// Only admins and moderators may see collapsed content.
func TestCollapsedComment_HiddenFromNonModerator(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	author := createTestUser(t, db, "student")
	mat := createTestMaterial(t, db)

	engRepo := repository.NewEngagementRepository(db)
	comment, err := engRepo.CreateComment(mat.ID, author.ID, "Collapsed content", 0)
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}
	// Collapse the comment directly (simulates hitting the 3-report threshold).
	if _, err := db.Exec(`UPDATE comments SET status = 'collapsed' WHERE id = ?`, comment.ID); err != nil {
		t.Fatalf("collapse comment: %v", err)
	}

	// Student requesting the material detail page must NOT see the collapsed comment.
	studentCookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodGet,
		fmt.Sprintf("/materials/%d", mat.ID), "", studentCookie, "")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on material detail, got %d", resp.StatusCode)
	}
	body := readBody(resp)
	if strings.Contains(body, "Collapsed content") {
		t.Error("collapsed comment body was visible to a non-moderator student — visibility leak")
	}

	// Moderator requesting the same page MUST see the collapsed comment.
	modCookie := loginAs(t, app, db, "moderator")
	resp2 := makeRequest(app, http.MethodGet,
		fmt.Sprintf("/materials/%d", mat.ID), "", modCookie, "")

	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for moderator on material detail, got %d", resp2.StatusCode)
	}
	body2 := readBody(resp2)
	if !strings.Contains(body2, "Collapsed content") {
		t.Error("collapsed comment was hidden from a moderator — moderators must see all comments")
	}
}

// TestModeration_ReCollapseAfterApproval verifies the full approve → re-report →
// re-collapse cycle works.  If ApproveComment sets 'active' instead of 'visible',
// the auto-collapse predicate (status='visible') silently fails and the comment
// can never be auto-collapsed again.
func TestModeration_ReCollapseAfterApproval(t *testing.T) {
	_, db, cleanup := newTestApp(t)
	defer cleanup()

	author := createTestUser(t, db, "student")
	mat := createTestMaterial(t, db)

	engRepo := repository.NewEngagementRepository(db)
	modRepo := repository.NewModerationRepository(db)

	// Create and collapse the comment.
	comment, err := engRepo.CreateComment(mat.ID, author.ID, "Borderline comment", 0)
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}
	if _, err := db.Exec(`UPDATE comments SET status = 'collapsed' WHERE id = ?`, comment.ID); err != nil {
		t.Fatalf("collapse comment: %v", err)
	}

	// Moderator approves → must return to 'visible'.
	if err := modRepo.ApproveComment(comment.ID, author.ID); err != nil {
		t.Fatalf("ApproveComment: %v", err)
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM comments WHERE id = ?`, comment.ID).Scan(&status); err != nil {
		t.Fatalf("query status: %v", err)
	}
	if status != "visible" {
		t.Fatalf("expected 'visible' after approve, got %q — re-collapse cycle broken", status)
	}

	// Three new reporters → auto-collapse must work again.
	reporters := []int64{}
	for i := 0; i < 3; i++ {
		u := createTestUser(t, db, "student")
		reporters = append(reporters, u.ID)
	}
	for _, rid := range reporters {
		if err := engRepo.ReportComment(comment.ID, rid, "inappropriate"); err != nil {
			t.Fatalf("ReportComment: %v", err)
		}
	}

	if err := db.QueryRow(`SELECT status FROM comments WHERE id = ?`, comment.ID).Scan(&status); err != nil {
		t.Fatalf("query status after re-report: %v", err)
	}
	if status != "collapsed" {
		t.Errorf("expected 'collapsed' after 3 new reports on previously-approved comment, got %q", status)
	}
}

// TestApproveComment_WrongStatus verifies that approving a comment that is not
// in 'collapsed' state returns 422 (the repository enforces status='collapsed').
func TestApproveComment_WrongStatus(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	author := createTestUser(t, db, "student")
	mat := createTestMaterial(t, db)

	engRepo := repository.NewEngagementRepository(db)
	comment, err := engRepo.CreateComment(mat.ID, author.ID, "Active comment", 0)
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}
	// comment is 'visible' — not collapsed; approve should fail.

	modCookie := loginAs(t, app, db, "moderator")

	resp := makeRequest(app, http.MethodPost,
		fmt.Sprintf("/moderation/%d/approve", comment.ID),
		"", modCookie, "application/x-www-form-urlencoded", htmxHeaders())

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 on approve of non-collapsed comment, got %d", resp.StatusCode)
	}
}
