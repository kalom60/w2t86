package repository_test

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"w2t86/internal/repository"
	"w2t86/internal/testutil"
)

// engagementFixtures creates a user and a material, returning their IDs.
func engagementFixtures(t *testing.T, db *sql.DB) (userID, materialID int64) {
	t.Helper()
	r, err := db.Exec(`INSERT INTO users (username, email, password_hash, role) VALUES ('enguser','eng@x.com','hash','student')`)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	userID, _ = r.LastInsertId()

	r2, err := db.Exec(`INSERT INTO materials (title, total_qty, available_qty, reserved_qty, status) VALUES ('Test Book', 10, 10, 0, 'active')`)
	if err != nil {
		t.Fatalf("insert material: %v", err)
	}
	materialID, _ = r2.LastInsertId()
	return
}

func newEngagementRepo(t *testing.T) (*repository.EngagementRepository, *sql.DB) {
	t.Helper()
	db := testutil.NewTestDB(t)
	return repository.NewEngagementRepository(db), db
}

func TestEngagementRepository_RecordVisit_And_GetHistory(t *testing.T) {
	repo, db := newEngagementRepo(t)
	userID, matID := engagementFixtures(t, db)

	if err := repo.RecordVisit(userID, matID); err != nil {
		t.Fatalf("RecordVisit: %v", err)
	}

	hist, err := repo.GetHistory(userID, 10)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(hist) != 1 {
		t.Errorf("expected 1 history entry, got %d", len(hist))
	}
	if hist[0].MaterialID != matID {
		t.Errorf("expected materialID=%d, got %d", matID, hist[0].MaterialID)
	}
}

func TestEngagementRepository_InsertRating_Once(t *testing.T) {
	repo, db := newEngagementRepo(t)
	userID, matID := engagementFixtures(t, db)

	if err := repo.InsertRating(matID, userID, 3); err != nil {
		t.Fatalf("InsertRating first: %v", err)
	}

	// Second attempt must return ErrAlreadyRated.
	err := repo.InsertRating(matID, userID, 5)
	if err == nil {
		t.Fatal("expected ErrAlreadyRated on second rating, got nil")
	}
	if !errors.Is(err, repository.ErrAlreadyRated) {
		t.Errorf("expected ErrAlreadyRated, got %v", err)
	}

	// Original rating must be unchanged.
	rating, err := repo.GetRating(matID, userID)
	if err != nil {
		t.Fatalf("GetRating: %v", err)
	}
	if rating == nil {
		t.Fatal("expected rating, got nil")
	}
	if rating.Stars != 3 {
		t.Errorf("expected stars=3 (original), got %d", rating.Stars)
	}
}

func TestEngagementRepository_GetAverageRating(t *testing.T) {
	repo, db := newEngagementRepo(t)
	_, matID := engagementFixtures(t, db)

	// Create two extra users for multi-rating test
	r1, err := db.Exec(`INSERT INTO users (username, email, password_hash, role) VALUES ('rater1','r1@x.com','h','student')`)
	if err != nil {
		t.Fatalf("insert rater1: %v", err)
	}
	uid1, _ := r1.LastInsertId()

	r2, err := db.Exec(`INSERT INTO users (username, email, password_hash, role) VALUES ('rater2','r2@x.com','h','student')`)
	if err != nil {
		t.Fatalf("insert rater2: %v", err)
	}
	uid2, _ := r2.LastInsertId()

	if err := repo.InsertRating(matID, uid1, 4); err != nil {
		t.Fatalf("InsertRating uid1: %v", err)
	}
	if err := repo.InsertRating(matID, uid2, 2); err != nil {
		t.Fatalf("InsertRating uid2: %v", err)
	}

	avg, count, err := repo.GetAverageRating(matID)
	if err != nil {
		t.Fatalf("GetAverageRating: %v", err)
	}
	if count != 2 {
		t.Errorf("expected count=2, got %d", count)
	}
	if avg != 3.0 {
		t.Errorf("expected avg=3.0, got %f", avg)
	}
}

func TestEngagementRepository_CreateComment_And_GetComments(t *testing.T) {
	repo, db := newEngagementRepo(t)
	userID, matID := engagementFixtures(t, db)

	c, err := repo.CreateComment(matID, userID, "Great book!", 0)
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	if c.ID == 0 {
		t.Fatal("expected non-zero comment ID")
	}

	comments, err := repo.GetComments(matID, false, 10, 0)
	if err != nil {
		t.Fatalf("GetComments: %v", err)
	}
	if len(comments) != 1 {
		t.Errorf("expected 1 comment, got %d", len(comments))
	}
	if comments[0].Body != "Great book!" {
		t.Errorf("unexpected body: %q", comments[0].Body)
	}
}

func TestEngagementRepository_ReportComment_AutoCollapseAt3(t *testing.T) {
	repo, db := newEngagementRepo(t)
	userID, matID := engagementFixtures(t, db)

	c, err := repo.CreateComment(matID, userID, "Controversial comment", 0)
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}

	// Create 3 distinct reporters
	var reporterIDs [3]int64
	for i := 0; i < 3; i++ {
		name := "reporter" + string(rune('1'+i))
		r, err := db.Exec(`INSERT INTO users (username, email, password_hash, role) VALUES (?,?,'h','student')`,
			name, name+"@x.com")
		if err != nil {
			t.Fatalf("insert reporter %d: %v", i, err)
		}
		reporterIDs[i], _ = r.LastInsertId()
	}

	// First two reports — status should remain visible
	for i := 0; i < 2; i++ {
		if err := repo.ReportComment(c.ID, reporterIDs[i], "spam"); err != nil {
			t.Fatalf("ReportComment %d: %v", i, err)
		}
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM comments WHERE id = ?`, c.ID).Scan(&status); err != nil {
		t.Fatalf("query status: %v", err)
	}
	if status != "visible" {
		t.Errorf("expected visible after 2 reports, got %q", status)
	}

	// Third report — should collapse
	if err := repo.ReportComment(c.ID, reporterIDs[2], "spam"); err != nil {
		t.Fatalf("ReportComment 3: %v", err)
	}
	if err := db.QueryRow(`SELECT status FROM comments WHERE id = ?`, c.ID).Scan(&status); err != nil {
		t.Fatalf("query status after 3 reports: %v", err)
	}
	if status != "collapsed" {
		t.Errorf("expected collapsed after 3 reports, got %q", status)
	}
}

func TestEngagementRepository_CreateList_And_AddItem(t *testing.T) {
	repo, db := newEngagementRepo(t)
	userID, matID := engagementFixtures(t, db)

	fl, err := repo.CreateList(userID, "My Favourites", "private")
	if err != nil {
		t.Fatalf("CreateList: %v", err)
	}
	if fl.ID == 0 {
		t.Fatal("expected non-zero list ID")
	}

	if err := repo.AddToList(fl.ID, matID); err != nil {
		t.Fatalf("AddToList: %v", err)
	}

	items, err := repo.GetListItems(fl.ID)
	if err != nil {
		t.Fatalf("GetListItems: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
	if items[0].MaterialID != matID {
		t.Errorf("expected materialID=%d, got %d", matID, items[0].MaterialID)
	}
}

func TestEngagementRepository_GenerateShareToken(t *testing.T) {
	repo, db := newEngagementRepo(t)
	userID, _ := engagementFixtures(t, db)

	fl, err := repo.CreateList(userID, "Shared List", "public")
	if err != nil {
		t.Fatalf("CreateList: %v", err)
	}

	expiry := time.Now().UTC().Add(24 * time.Hour)
	token, err := repo.GenerateShareToken(fl.ID, expiry)
	if err != nil {
		t.Fatalf("GenerateShareToken: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	// Should be retrievable by token
	found, err := repo.GetListByShareToken(token)
	if err != nil {
		t.Fatalf("GetListByShareToken: %v", err)
	}
	if found.ID != fl.ID {
		t.Errorf("expected list ID=%d, got %d", fl.ID, found.ID)
	}
}

func TestEngagementRepository_CountRecentComments_RateLimit(t *testing.T) {
	repo, db := newEngagementRepo(t)
	userID, matID := engagementFixtures(t, db)

	// Post 3 comments right now.
	for i := 0; i < 3; i++ {
		if _, err := repo.CreateComment(matID, userID, "comment", 0); err != nil {
			t.Fatalf("CreateComment %d: %v", i, err)
		}
	}

	// A cutoff 5 minutes ago should see all 3 comments (they were just created).
	since := time.Now().Add(-5 * time.Minute)
	count, err := repo.CountRecentComments(userID, since)
	if err != nil {
		t.Fatalf("CountRecentComments: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 recent comments within 5-minute window, got %d", count)
	}

	// A cutoff in the future should yield zero results.
	futureSince := time.Now().Add(1 * time.Minute)
	count2, err := repo.CountRecentComments(userID, futureSince)
	if err != nil {
		t.Fatalf("CountRecentComments future: %v", err)
	}
	if count2 != 0 {
		t.Errorf("expected 0 comments with future cutoff, got %d", count2)
	}
}
