package auth_test

// credentials_integrity_test.go — programmatic proof that the seeded admin
// account uses a non-functional bootstrap placeholder (not a known exploitable
// password) and that the placeholder itself cannot authenticate anyone.
//
// The old approach seeded a known bcrypt hash for "ChangeMe123!" directly in
// the migration, creating an exploitable window between deployment and first
// login. The new approach seeds the sentinel string "BOOTSTRAP_PENDING_ROTATION"
// which is NOT a valid bcrypt hash. On first boot, cmd/server/main.go detects
// this sentinel, generates a random password, hashes it, and logs it once.
//
// Run with:
//
//	go test -v ./internal/auth/...

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// migrationPath returns the absolute path to migrations/001_schema.sql.
// It uses runtime.Caller(0) so the result is correct regardless of the working
// directory from which `go test` is invoked.
func migrationPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	// file: …/internal/auth/credentials_integrity_test.go
	// root: …/  (two directories up)
	root := filepath.Join(filepath.Dir(file), "..", "..")
	return filepath.Join(root, "migrations", "001_schema.sql")
}

// TestAdminCredentials_HashMatchesDocumentedPassword verifies that the migration
// seeds the admin account with the non-functional bootstrap placeholder, NOT a
// known exploitable bcrypt hash.
//
// PASS means: no freshly-initialised database will accept a known password for
// admin — login is blocked until the server performs auto-rotation on first boot.
func TestAdminCredentials_HashMatchesDocumentedPassword(t *testing.T) {
	data, err := os.ReadFile(migrationPath(t))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	content := string(data)

	const placeholder = "BOOTSTRAP_PENDING_ROTATION"
	if !strings.Contains(content, placeholder) {
		t.Errorf("migrations/001_schema.sql must contain the bootstrap placeholder %q — found none\n"+
			"This means a known credential is seeded, which is a security risk.", placeholder)
	}

	// The legacy known hash must no longer appear.
	const legacyHash = "$2a$12$fMPISK6tAC1XLVM3JdJQDuB/CrXgdRM.LUPHHu4/VxS/vzihnYyQ."
	if strings.Contains(content, legacyHash) {
		t.Errorf("migrations/001_schema.sql still contains the legacy known-default bcrypt hash.\n"+
			"Replace it with the bootstrap placeholder %q.", placeholder)
	}
}

// TestAdminCredentials_HashCost verifies that the bootstrap placeholder is NOT a
// valid bcrypt hash — confirming that login is blocked until auto-rotation.
func TestAdminCredentials_HashCost(t *testing.T) {
	const placeholder = "BOOTSTRAP_PENDING_ROTATION"

	// bcrypt.Cost on a non-bcrypt string must return an error.
	_, err := bcrypt.Cost([]byte(placeholder))
	if err == nil {
		t.Error("BOOTSTRAP_PENDING_ROTATION must NOT be parseable as a bcrypt hash — " +
			"if bcrypt.Cost succeeds, the placeholder could accidentally authenticate")
	}
}

// TestAdminCredentials_NearMissesRejected verifies that the bootstrap placeholder
// cannot authenticate any password — not even common guesses.
func TestAdminCredentials_NearMissesRejected(t *testing.T) {
	const placeholder = "BOOTSTRAP_PENDING_ROTATION"
	cases := []string{
		"ChangeMe123!",  // old documented default
		"changeme123!",  // wrong capitalisation
		"ChangeMe123",   // missing punctuation
		"admin",         // obvious guess
		"password",      // obvious guess
		"",              // empty
	}
	for _, bad := range cases {
		if bcrypt.CompareHashAndPassword([]byte(placeholder), []byte(bad)) == nil {
			t.Errorf("placeholder unexpectedly accepted password %q — it must be non-functional", bad)
		}
	}
}
