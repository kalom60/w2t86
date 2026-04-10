package repository_test

// admin_credential_test.go — programmatic proof that the seeded admin account
// uses a non-functional bootstrap placeholder (not a known exploitable password)
// and that the server's auto-rotation mechanism produces a working bcrypt hash.
//
// The old approach seeded a known bcrypt hash for "ChangeMe123!" directly in the
// migration, creating an exploitable window between deployment and first login.
// The new approach seeds the sentinel string "BOOTSTRAP_PENDING_ROTATION" which
// is NOT a valid bcrypt hash.  On first boot, cmd/server/main.go detects this
// sentinel, generates a random password, hashes it, and logs it once.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"w2t86/internal/crypto"
)

// schemaFilePath returns the absolute path to migrations/001_schema.sql by
// navigating upward from this source file's location.
func schemaFilePath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed — cannot determine source file location")
	}
	root := filepath.Join(filepath.Dir(thisFile), "..", "..")
	return filepath.Join(root, "migrations", "001_schema.sql")
}

// TestAdminSeed_UsesBootstrapPlaceholder verifies that the migration file does
// NOT contain a known exploitable bcrypt hash for the admin account.
// The seeded value must be the sentinel "BOOTSTRAP_PENDING_ROTATION" so that
// login is impossible until the server performs the auto-rotation on first boot.
func TestAdminSeed_UsesBootstrapPlaceholder(t *testing.T) {
	data, err := os.ReadFile(schemaFilePath(t))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	content := string(data)

	const placeholder = "BOOTSTRAP_PENDING_ROTATION"
	if !strings.Contains(content, placeholder) {
		t.Errorf("migrations/001_schema.sql must contain the bootstrap placeholder %q — found none\n"+
			"This means a known credential is seeded, which is a security risk.", placeholder)
	}
}

// TestAdminSeed_NoKnownBcryptHash verifies that the migration does NOT contain
// a known exploitable bcrypt hash (the old "ChangeMe123!" hash or any other
// well-known hash that would let an attacker log in before auto-rotation).
func TestAdminSeed_NoKnownBcryptHash(t *testing.T) {
	data, err := os.ReadFile(schemaFilePath(t))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	content := string(data)

	// The well-known legacy hash for "ChangeMe123!" must no longer appear in the
	// migration source.
	const legacyHash = "$2a$12$fMPISK6tAC1XLVM3JdJQDuB/CrXgdRM.LUPHHu4/VxS/vzihnYyQ."
	if strings.Contains(content, legacyHash) {
		t.Errorf("migrations/001_schema.sql still contains the legacy known-default bcrypt hash.\n"+
			"Replace it with the bootstrap placeholder %q.", "BOOTSTRAP_PENDING_ROTATION")
	}
}

// TestAutoRotation_ProducesWorkingHash verifies that the bootstrap rotation
// logic (GenerateRandomPassword + HashPassword + CheckPassword) produces a
// valid, usable bcrypt credential — i.e., the round-trip works correctly.
func TestAutoRotation_ProducesWorkingHash(t *testing.T) {
	pass, err := crypto.GenerateRandomPassword()
	if err != nil {
		t.Fatalf("GenerateRandomPassword: %v", err)
	}
	if len(pass) < 16 {
		t.Errorf("generated password too short: %d chars", len(pass))
	}

	hash, err := crypto.HashPassword(pass)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !crypto.CheckPassword(hash, pass) {
		t.Error("CheckPassword returned false for a freshly generated password — rotation would produce an unusable credential")
	}
}
