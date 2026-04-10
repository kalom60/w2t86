package unit_tests

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"w2t86/internal/config"
	"w2t86/internal/repository"
	"w2t86/internal/services"
	"w2t86/internal/testutil"
)

// newAuthService returns a wired AuthService backed by a fresh test DB.
func newAuthService(t *testing.T) *services.AuthService {
	t.Helper()
	db := testutil.NewTestDB(t)
	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db)
	cfg := &config.Config{}
	return services.NewAuthService(userRepo, sessionRepo, cfg)
}

// registerUser is a convenience helper for auth tests.
func registerUser(t *testing.T, svc *services.AuthService, username, password string) {
	t.Helper()
	_, err := svc.Register(username, username+"@example.com", password, "student")
	if err != nil {
		t.Fatalf("registerUser(%q): %v", username, err)
	}
}

// hashTokenForTest replicates the internal HMAC-SHA256 hashToken logic so we
// can query sessions directly without exposing the function from the services
// package. secret must match the SessionSecret used by the AuthService under test.
func hashTokenForTest(secret, token string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestAuth_Register_MinPasswordLength_Exactly12_Passes(t *testing.T) {
	svc := newAuthService(t)
	_, err := svc.Register("usermin12", "m@x.com", "123456789012", "student")
	if err != nil {
		t.Errorf("expected registration with exactly 12-char password to succeed, got: %v", err)
	}
}

func TestAuth_Register_PasswordLength_11_Fails(t *testing.T) {
	svc := newAuthService(t)
	_, err := svc.Register("user11", "u11@x.com", "12345678901", "student")
	if err == nil {
		t.Error("expected error for 11-char password, got nil")
	}
}

func TestAuth_Register_PasswordLength_50_Passes(t *testing.T) {
	svc := newAuthService(t)
	password := strings.Repeat("a", 50)
	_, err := svc.Register("user50", "u50@x.com", password, "student")
	if err != nil {
		t.Errorf("expected 50-char password to pass, got: %v", err)
	}
}

func TestAuth_Register_DuplicateUsername_Fails(t *testing.T) {
	svc := newAuthService(t)
	const username = "dupuser"
	registerUser(t, svc, username, "password_12345")
	_, err := svc.Register(username, "other@x.com", "password_12345", "student")
	if err == nil {
		t.Error("expected error for duplicate username, got nil")
	}
}

func TestAuth_Login_CorrectCredentials_Succeeds(t *testing.T) {
	svc := newAuthService(t)
	registerUser(t, svc, "loginok", "goodpassword1")

	token, user, err := svc.Login("loginok", "goodpassword1")
	if err != nil {
		t.Fatalf("expected successful login, got: %v", err)
	}
	if token == "" {
		t.Error("expected non-empty token")
	}
	if user == nil {
		t.Error("expected non-nil user")
	}
}

func TestAuth_Login_WrongPassword_Fails(t *testing.T) {
	svc := newAuthService(t)
	registerUser(t, svc, "loginwrong", "correctpassword1")

	_, _, err := svc.Login("loginwrong", "wrongpassword1")
	if err == nil {
		t.Error("expected error for wrong password, got nil")
	}
}

func TestAuth_Login_WrongPassword_IncrementsFailedAttempts(t *testing.T) {
	db := testutil.NewTestDB(t)
	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db)
	cfg := &config.Config{}
	svc := services.NewAuthService(userRepo, sessionRepo, cfg)

	_, err := svc.Register("failinc", "fi@x.com", "password12345", "student")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	svc.Login("failinc", "wrongpassword") //nolint:errcheck

	user, err := userRepo.GetByUsername("failinc")
	if err != nil {
		t.Fatalf("GetByUsername: %v", err)
	}
	if user.FailedAttempts != 1 {
		t.Errorf("expected failed_attempts=1, got %d", user.FailedAttempts)
	}
}

func TestAuth_Login_LockoutAfterExactlyFiveFailures(t *testing.T) {
	db := testutil.NewTestDB(t)
	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db)
	cfg := &config.Config{}
	svc := services.NewAuthService(userRepo, sessionRepo, cfg)

	_, err := svc.Register("lockout5", "lo5@x.com", "password12345", "student")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	for i := 0; i < 5; i++ {
		svc.Login("lockout5", "wrongpassword") //nolint:errcheck
	}

	user, err := userRepo.GetByUsername("lockout5")
	if err != nil {
		t.Fatalf("GetByUsername: %v", err)
	}
	if user.LockedUntil == nil {
		t.Error("expected account to be locked after 5 failures, locked_until is nil")
	}
}

func TestAuth_Login_LockedAccount_RejectsCorrectPassword(t *testing.T) {
	svc := newAuthService(t)
	registerUser(t, svc, "locked_acc", "password12345")

	// Trigger lockout.
	for i := 0; i < 5; i++ {
		svc.Login("locked_acc", "wrongpassword") //nolint:errcheck
	}

	_, _, err := svc.Login("locked_acc", "password12345")
	if err == nil {
		t.Error("expected error for locked account with correct password, got nil")
	}
	if !strings.Contains(err.Error(), "locked") {
		t.Errorf("expected 'locked' in error message, got: %v", err)
	}
}

func TestAuth_Login_SuccessAfterFailures_ResetCounter(t *testing.T) {
	db := testutil.NewTestDB(t)
	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db)
	cfg := &config.Config{}
	svc := services.NewAuthService(userRepo, sessionRepo, cfg)

	_, err := svc.Register("resetctr", "rc@x.com", "password12345", "student")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// 3 failed attempts.
	for i := 0; i < 3; i++ {
		svc.Login("resetctr", "wrongpassword") //nolint:errcheck
	}

	// Successful login.
	_, _, err = svc.Login("resetctr", "password12345")
	if err != nil {
		t.Fatalf("expected successful login, got: %v", err)
	}

	user, err := userRepo.GetByUsername("resetctr")
	if err != nil {
		t.Fatalf("GetByUsername: %v", err)
	}
	if user.FailedAttempts != 0 {
		t.Errorf("expected failed_attempts=0 after successful login, got %d", user.FailedAttempts)
	}
}

func TestAuth_Logout_InvalidatesSession(t *testing.T) {
	db := testutil.NewTestDB(t)
	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db)
	cfg := &config.Config{}
	svc := services.NewAuthService(userRepo, sessionRepo, cfg)

	_, err := svc.Register("logoutuser", "lo@x.com", "password12345", "student")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	token, _, err := svc.Login("logoutuser", "password12345")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	if err := svc.Logout(token); err != nil {
		t.Fatalf("logout: %v", err)
	}

	// Verify the session row is gone by looking up the token hash.
	// The test AuthService uses cfg.SessionSecret="" (empty string).
	tokenHash := hashTokenForTest("", token)
	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sessions WHERE token_hash = ?`, tokenHash,
	).Scan(&count); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if count != 0 {
		t.Errorf("expected session to be deleted after logout, but found %d row(s)", count)
	}
}

func TestAuth_Session_Token_IsUnique(t *testing.T) {
	svc := newAuthService(t)
	registerUser(t, svc, "tokenuniq", "password12345")

	token1, _, err1 := svc.Login("tokenuniq", "password12345")
	token2, _, err2 := svc.Login("tokenuniq", "password12345")
	if err1 != nil || err2 != nil {
		t.Fatalf("login errors: %v / %v", err1, err2)
	}
	if token1 == token2 {
		t.Error("expected two different session tokens, got identical tokens")
	}
}

func TestAuth_Session_Expiry_StillValid(t *testing.T) {
	db := testutil.NewTestDB(t)
	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db)
	cfg := &config.Config{}
	svc := services.NewAuthService(userRepo, sessionRepo, cfg)

	_, err := svc.Register("sessvalid", "sv@x.com", "password12345", "student")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	token, _, err := svc.Login("sessvalid", "password12345")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	// The session was just created — should still be present and valid (within 24h).
	tokenHash := hashTokenForTest("", token)
	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sessions WHERE token_hash = ? AND expires_at > datetime('now')`,
		tokenHash,
	).Scan(&count); err != nil {
		t.Fatalf("count valid sessions: %v", err)
	}
	if count == 0 {
		t.Errorf("expected a valid session within 24h for token starting %q, but none found",
			fmt.Sprintf("%.8s…", token))
	}
}

// ---------------------------------------------------------------------------
// Session tamper-detection: HMAC key separation
// ---------------------------------------------------------------------------

// TestAuth_SessionHash_DependsOnSecret verifies that the token hash stored in
// the sessions table is HMAC-keyed with SESSION_SECRET, not a bare SHA-256.
// A hash computed with a different secret must not match any DB row, so an
// attacker who knows the raw token but not the secret cannot construct a valid
// session hash.
func TestAuth_SessionHash_DependsOnSecret(t *testing.T) {
	const correctSecret = "correct-session-secret-for-test"
	const wrongSecret   = "attacker-does-not-know-real-secret"

	db := testutil.NewTestDB(t)
	userRepo    := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db)
	cfg         := &config.Config{SessionSecret: correctSecret}
	svc         := services.NewAuthService(userRepo, sessionRepo, cfg)

	if _, err := svc.Register("hmacuser", "hmac@x.com", "password12345", "student"); err != nil {
		t.Fatalf("register: %v", err)
	}

	token, _, err := svc.Login("hmacuser", "password12345")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	// A hash computed with the WRONG secret must not find the session.
	wrongHash := hashTokenForTest(wrongSecret, token)
	var wrongCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sessions WHERE token_hash = ?`, wrongHash,
	).Scan(&wrongCount); err != nil {
		t.Fatalf("count (wrong secret): %v", err)
	}
	if wrongCount != 0 {
		t.Error("session was found using a hash computed with the WRONG secret — HMAC key is not enforced")
	}

	// A hash computed with the CORRECT secret must find exactly one session.
	correctHash := hashTokenForTest(correctSecret, token)
	var correctCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sessions WHERE token_hash = ?`, correctHash,
	).Scan(&correctCount); err != nil {
		t.Fatalf("count (correct secret): %v", err)
	}
	if correctCount != 1 {
		t.Errorf("expected 1 session with correct secret hash, got %d", correctCount)
	}
}
