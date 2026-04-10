package services

import (
	"fmt"

	"w2t86/internal/crypto"
	"w2t86/internal/models"
	"w2t86/internal/observability"
	"w2t86/internal/repository"
)

// AdminService orchestrates admin-level operations: user management,
// custom fields, duplicate detection / merging, and audit log access.
type AdminService struct {
	adminRepo    *repository.AdminRepository
	userRepo     *repository.UserRepository
	materialRepo *repository.MaterialRepository
}

// NewAdminService creates an AdminService wired to the given repositories.
func NewAdminService(
	ar *repository.AdminRepository,
	ur *repository.UserRepository,
	mr *repository.MaterialRepository,
) *AdminService {
	return &AdminService{adminRepo: ar, userRepo: ur, materialRepo: mr}
}

// ---------------------------------------------------------------
// User admin
// ---------------------------------------------------------------

// ListUsers returns paginated users optionally filtered by role.
func (s *AdminService) ListUsers(role string, limit, offset int) ([]models.User, error) {
	return s.adminRepo.ListUsers(role, limit, offset)
}

// CreateUser registers a new user account.  fullName is the person's real
// name used for duplicate detection; pass an empty string to leave it unset.
// encKey, if 32 bytes, encrypts fullName before storage.
func (s *AdminService) CreateUser(username, email, password, role, fullName string, encKey []byte) (*models.User, error) {
	authSvc := &AuthService{userRepo: s.userRepo}
	user, err := authSvc.Register(username, email, password, role)
	if err != nil {
		return nil, fmt.Errorf("service: AdminService.CreateUser: %w", err)
	}
	if fullName != "" {
		stored, err := encryptSensitiveField(encKey, fullName)
		if err != nil {
			return nil, fmt.Errorf("service: AdminService.CreateUser: encrypt full_name: %w", err)
		}
		idx := crypto.BlindIndex(encKey, fullName)
		phonetic := crypto.Soundex(fullName)
		if err := s.userRepo.SetFullName(user.ID, stored, idx, phonetic); err != nil {
			// Non-fatal: user was created; log and continue.
			observability.App.Warn("set full_name failed", "user_id", user.ID, "error", err)
		} else {
			user.FullName = &fullName // expose plaintext to caller
		}
	}
	return user, nil
}

// SetUserFullName encrypts fullName and stores the value along with its HMAC
// blind index.  encKey must be exactly 32 bytes; the call fails if it is not.
func (s *AdminService) SetUserFullName(userID int64, fullName string, encKey []byte) error {
	stored, err := encryptSensitiveField(encKey, fullName)
	if err != nil {
		return fmt.Errorf("service: AdminService.SetUserFullName: %w", err)
	}
	idx := crypto.BlindIndex(encKey, fullName)
	phonetic := crypto.Soundex(fullName)
	return s.userRepo.SetFullName(userID, stored, idx, phonetic)
}

// SetUserExternalID encrypts externalID and stores the value along with its HMAC
// blind index.  encKey must be exactly 32 bytes; the call fails if it is not.
func (s *AdminService) SetUserExternalID(userID int64, externalID string, encKey []byte) error {
	stored, err := encryptSensitiveField(encKey, externalID)
	if err != nil {
		return fmt.Errorf("service: AdminService.SetUserExternalID: %w", err)
	}
	idx := crypto.BlindIndex(encKey, externalID)
	return s.userRepo.SetExternalID(userID, stored, idx)
}

// DecryptUser decrypts the sensitive fields (full_name, external_id,
// date_of_birth) on a copy of the user.  If a field is not encrypted (legacy
// plaintext row) or decryption fails, the stored value is returned as-is.
func (s *AdminService) DecryptUser(user *models.User, encKey []byte) *models.User {
	if user == nil {
		return nil
	}
	out := *user // shallow copy — only pointer fields differ
	if user.FullName != nil {
		plain := decryptSensitiveField(encKey, *user.FullName)
		out.FullName = &plain
	}
	if user.ExternalID != nil {
		plain := decryptSensitiveField(encKey, *user.ExternalID)
		out.ExternalID = &plain
	}
	if user.DateOfBirth != nil {
		plain := decryptSensitiveField(encKey, *user.DateOfBirth)
		out.DateOfBirth = &plain
	}
	return &out
}

// encryptedPrefix is prepended to ciphertext so that legacy plaintext rows
// can be distinguished from encrypted values without a separate flag column.
const encryptedPrefix = "enc:"

// encryptSensitiveField encrypts value with AES-256-GCM and prepends the
// encryptedPrefix sentinel.  When encKey is absent or invalid the plaintext is
// returned unchanged so that callers without a key still write readable data.
func encryptSensitiveField(encKey []byte, value string) (string, error) {
	if len(encKey) != 32 {
		return "", fmt.Errorf("encryption key is missing or invalid (%d bytes); refusing plaintext write of sensitive field", len(encKey))
	}
	ct, err := crypto.EncryptField(encKey, value)
	if err != nil {
		return "", err
	}
	return encryptedPrefix + ct, nil
}

// decryptSensitiveField strips the encryptedPrefix and decrypts.  If the
// value does not start with the prefix it is returned unchanged (legacy row).
func decryptSensitiveField(encKey []byte, stored string) string {
	if len(encKey) != 32 || !hasEncPrefix(stored) {
		return stored
	}
	plain, err := crypto.DecryptField(encKey, stored[len(encryptedPrefix):])
	if err != nil {
		return stored // decryption failure: surface raw (shouldn't happen in prod)
	}
	return plain
}

func hasEncPrefix(s string) bool {
	return len(s) > len(encryptedPrefix) && s[:len(encryptedPrefix)] == encryptedPrefix
}

// UpdateUserRole changes a user's role and writes an audit log entry.
func (s *AdminService) UpdateUserRole(userID int64, role string, actorID int64, actorIP string) error {
	// Capture before state.
	before, _ := s.userRepo.GetByID(userID)

	if err := s.adminRepo.UpdateUserRole(userID, role); err != nil {
		return fmt.Errorf("service: AdminService.UpdateUserRole: %w", err)
	}

	// Write audit log (best-effort).
	after, _ := s.userRepo.GetByID(userID)
	_ = s.adminRepo.WriteAuditLog(actorID, "update_role", "user", userID, before, after, actorIP)
	observability.Security.Info("role changed", "target_user_id", userID, "new_role", role, "actor_id", actorID)
	return nil
}

// UnlockUser clears a user's account lockout.
func (s *AdminService) UnlockUser(userID int64, actorID int64) error {
	if err := s.adminRepo.UnlockUser(userID); err != nil {
		return fmt.Errorf("service: AdminService.UnlockUser: %w", err)
	}
	_ = s.adminRepo.WriteAuditLog(actorID, "unlock", "user", userID, nil, nil, "")
	observability.Security.Info("account unlocked", "target_user_id", userID, "actor_id", actorID)
	return nil
}

// SetCustomField stores a custom field for the given entity.
// If encrypt is true the value is AES-256-GCM encrypted with encKey before
// being stored; encKey must be exactly 32 bytes.  If encryption is requested
// but the key is absent or the wrong length, the call fails rather than
// silently falling back to plaintext storage.
// actorID and reason are written to the immutable audit trail.
func (s *AdminService) SetCustomField(entityType string, entityID int64, name, value string, encrypt bool, encKey []byte, actorID int64, reason string) error {
	stored := value
	if encrypt {
		if len(encKey) != 32 {
			return fmt.Errorf("service: AdminService.SetCustomField: encryption requested but key is invalid or missing (got %d bytes, need 32)", len(encKey))
		}
		enc, err := crypto.EncryptField(encKey, value)
		if err != nil {
			return fmt.Errorf("service: AdminService.SetCustomField: encrypt: %w", err)
		}
		stored = enc
	}
	return s.adminRepo.SetCustomField(entityType, entityID, name, stored, encrypt, actorID, reason)
}

// GetCustomFields returns all custom fields for the given entity.
// Encrypted fields have their FieldValue replaced with the plaintext if encKey
// is provided and 32 bytes; otherwise the raw (ciphertext) value is returned.
func (s *AdminService) GetCustomFields(entityType string, entityID int64, encKey []byte) ([]models.EntityCustomField, error) {
	fields, err := s.adminRepo.GetCustomFields(entityType, entityID)
	if err != nil {
		return nil, err
	}
	if len(encKey) == 32 {
		for i, f := range fields {
			if f.IsEncrypted == 1 && f.FieldValue != nil {
				plain, decErr := crypto.DecryptField(encKey, *f.FieldValue)
				if decErr == nil {
					fields[i].FieldValue = &plain
				}
				// If decryption fails leave the raw value; caller can decide.
			}
		}
	}
	return fields, nil
}

// DeleteCustomField removes a custom field for the given entity.
// actorID and reason are written to the immutable audit trail.
func (s *AdminService) DeleteCustomField(entityType string, entityID int64, name string, actorID int64, reason string) error {
	return s.adminRepo.DeleteCustomField(entityType, entityID, name, actorID, reason)
}

// GetCustomFieldAuditLog returns the immutable audit trail for a specific entity's custom fields.
func (s *AdminService) GetCustomFieldAuditLog(entityType string, entityID int64) ([]models.EntityCustomFieldAudit, error) {
	return s.adminRepo.GetCustomFieldAuditLog(entityType, entityID)
}

// GetEntityDisplayName returns a human-readable label for the entity
// (e.g. the user's username, a material's title, a course name).
// Never returns an error; falls back to "<type> #<id>" for unknown rows.
func (s *AdminService) GetEntityDisplayName(entityType string, entityID int64) string {
	return s.adminRepo.GetEntityDisplayName(entityType, entityID)
}

// ---------------------------------------------------------------
// Duplicate detection
// ---------------------------------------------------------------

// FindDuplicates returns potential duplicate user pairs.
func (s *AdminService) FindDuplicates() ([]repository.DuplicatePair, error) {
	return s.adminRepo.FindDuplicateUsers(100)
}

// MergeUsers merges duplicateID into primaryID and records the operation.
func (s *AdminService) MergeUsers(primaryID, duplicateID int64, actorID int64) error {
	if err := s.adminRepo.MergeUsers(primaryID, duplicateID, actorID); err != nil {
		return fmt.Errorf("service: AdminService.MergeUsers: %w", err)
	}
	_ = s.adminRepo.WriteAuditLog(actorID, "merge_user", "user", primaryID,
		map[string]int64{"duplicate_id": duplicateID}, nil, "")
	observability.Security.Warn("entities merged", "primary_id", primaryID, "duplicate_id", duplicateID, "actor_id", actorID)
	return nil
}

// ---------------------------------------------------------------
// Audit log
// ---------------------------------------------------------------

// GetAuditLog returns paginated audit log entries for a specific entity.
func (s *AdminService) GetAuditLog(entityType string, entityID int64, limit, offset int) ([]models.AuditLog, error) {
	return s.adminRepo.GetAuditLog(entityType, entityID, limit, offset)
}

// GetRecentAuditLog returns the most-recent limit audit log entries.
func (s *AdminService) GetRecentAuditLog(limit int) ([]models.AuditLog, error) {
	return s.adminRepo.GetRecentAuditLog(limit)
}
