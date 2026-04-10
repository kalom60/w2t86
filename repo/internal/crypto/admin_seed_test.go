package crypto_test

// admin_seed_test.go — verifies that the default admin account seeded in
// migrations/001_schema.sql uses a non-functional bootstrap placeholder (not a
// known exploitable password) and that the auto-rotation mechanism produces a
// valid bcrypt credential at cost 12.
//
// The old approach seeded a known bcrypt hash for "ChangeMe123!" directly in
// the migration, creating an exploitable window between deployment and first
// login. The new approach seeds the sentinel string "BOOTSTRAP_PENDING_ROTATION"
// which is NOT a valid bcrypt hash. On first boot, cmd/server/main.go detects
// this sentinel, generates a random password, hashes it, and logs it once.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"w2t86/internal/crypto"
)

// schemaPath resolves the absolute path to migrations/001_schema.sql
// relative to this source file, so the test works regardless of the working
// directory from which `go test` is invoked.
func schemaPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed — cannot determine source file path")
	}
	// thisFile: .../internal/crypto/admin_seed_test.go
	// root:     .../  (two directories up)
	root := filepath.Join(filepath.Dir(thisFile), "..", "..")
	return filepath.Join(root, "migrations", "001_schema.sql")
}

// TestDefaultAdminPassword_MatchesSeedHash verifies that the migration seeds
// the admin account with the non-functional bootstrap placeholder rather than a
// known exploitable bcrypt hash.
//
// PASS means: no freshly-seeded database accepts a documented default password
// — login is blocked until the server's auto-rotation runs on first boot.
func TestDefaultAdminPassword_MatchesSeedHash(t *testing.T) {
	data, err := os.ReadFile(schemaPath(t))
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

// TestDefaultAdminPassword_SeedHashIsBcryptCost12 verifies that the bootstrap
// auto-rotation (GenerateRandomPassword + HashPassword) produces a bcrypt hash
// at cost 12 — satisfying the password-hashing security policy.
func TestDefaultAdminPassword_SeedHashIsBcryptCost12(t *testing.T) {
	pass, err := crypto.GenerateRandomPassword()
	if err != nil {
		t.Fatalf("GenerateRandomPassword: %v", err)
	}

	hash, err := crypto.HashPassword(pass)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	// bcrypt hash format: $2a$<cost>$...
	// HashPassword uses cost 12 → "$2a$12$"
	if !strings.HasPrefix(hash, "$2a$12$") {
		t.Errorf("HashPassword did not produce a bcrypt cost-12 hash: %s", hash)
	}

	// Round-trip must succeed.
	if !crypto.CheckPassword(hash, pass) {
		t.Error("CheckPassword returned false for a freshly generated password — rotation would produce an unusable credential")
	}
}
