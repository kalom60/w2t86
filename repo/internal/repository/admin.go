package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"w2t86/internal/models"
)

// AdminRepository provides database operations for admin-specific features:
// custom user fields, duplicate detection/merging, audit logs, and user
// management actions that are too elevated for the regular UserRepository.
type AdminRepository struct {
	db *sql.DB
}

// NewAdminRepository creates an AdminRepository backed by the given database.
func NewAdminRepository(db *sql.DB) *AdminRepository {
	return &AdminRepository{db: db}
}

// ---------------------------------------------------------------
// Custom fields
// ---------------------------------------------------------------

// SetCustomField upserts a custom field for the given entity and writes an
// immutable audit record capturing who changed the field, when, and why.
// actorID is the ID of the admin performing the mutation; reason is mandatory.
func (r *AdminRepository) SetCustomField(entityType string, entityID int64, fieldName, fieldValue string, isEncrypted bool, actorID int64, reason string) error {
	enc := 0
	if isEncrypted {
		enc = 1
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("repository: SetCustomField: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Read old value before the upsert for the audit trail.
	var oldValue *string
	var oldIsEncrypted int
	readErr := tx.QueryRow(
		`SELECT field_value, is_encrypted FROM entity_custom_fields WHERE entity_type = ? AND entity_id = ? AND field_name = ?`,
		entityType, entityID, fieldName,
	).Scan(&oldValue, &oldIsEncrypted)
	if readErr != nil && readErr != sql.ErrNoRows {
		return fmt.Errorf("repository: SetCustomField: read old value: %w", readErr)
	}

	const upsertQ = `
		INSERT INTO entity_custom_fields (entity_type, entity_id, field_name, field_value, is_encrypted)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(entity_type, entity_id, field_name) DO UPDATE
		SET field_value  = excluded.field_value,
		    is_encrypted = excluded.is_encrypted`
	if _, err := tx.Exec(upsertQ, entityType, entityID, fieldName, fieldValue, enc); err != nil {
		return fmt.Errorf("repository: SetCustomField: upsert: %w", err)
	}

	const auditQ = `
		INSERT INTO entity_custom_fields_audit
		            (entity_type, entity_id, field_name, old_value, new_value, is_encrypted, actor_id, reason)
		VALUES      (?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := tx.Exec(auditQ, entityType, entityID, fieldName, oldValue, fieldValue, enc, actorID, reason); err != nil {
		return fmt.Errorf("repository: SetCustomField: write audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repository: SetCustomField: commit: %w", err)
	}
	return nil
}

// GetCustomFields returns all custom fields for the given entity.
func (r *AdminRepository) GetCustomFields(entityType string, entityID int64) ([]models.EntityCustomField, error) {
	const q = `
		SELECT id, entity_type, entity_id, field_name, field_value, is_encrypted
		FROM   entity_custom_fields
		WHERE  entity_type = ? AND entity_id = ?
		ORDER  BY field_name`

	rows, err := r.db.Query(q, entityType, entityID)
	if err != nil {
		return nil, fmt.Errorf("repository: GetCustomFields: %w", err)
	}
	defer rows.Close()

	var out []models.EntityCustomField
	for rows.Next() {
		var f models.EntityCustomField
		if err := rows.Scan(&f.ID, &f.EntityType, &f.EntityID, &f.FieldName, &f.FieldValue, &f.IsEncrypted); err != nil {
			return nil, fmt.Errorf("repository: GetCustomFields: scan: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// DeleteCustomField removes a specific custom field and writes an audit record.
// actorID is the ID of the admin performing the deletion; reason is mandatory.
func (r *AdminRepository) DeleteCustomField(entityType string, entityID int64, fieldName string, actorID int64, reason string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("repository: DeleteCustomField: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Read old value for the audit trail before deleting.
	var oldValue *string
	var oldIsEncrypted int
	readErr := tx.QueryRow(
		`SELECT field_value, is_encrypted FROM entity_custom_fields WHERE entity_type = ? AND entity_id = ? AND field_name = ?`,
		entityType, entityID, fieldName,
	).Scan(&oldValue, &oldIsEncrypted)
	if readErr != nil && readErr != sql.ErrNoRows {
		return fmt.Errorf("repository: DeleteCustomField: read old value: %w", readErr)
	}

	if _, err := tx.Exec(
		`DELETE FROM entity_custom_fields WHERE entity_type = ? AND entity_id = ? AND field_name = ?`,
		entityType, entityID, fieldName,
	); err != nil {
		return fmt.Errorf("repository: DeleteCustomField: delete: %w", err)
	}

	const auditQ = `
		INSERT INTO entity_custom_fields_audit
		            (entity_type, entity_id, field_name, old_value, new_value, is_encrypted, actor_id, reason)
		VALUES      (?, ?, ?, ?, NULL, ?, ?, ?)`
	if _, err := tx.Exec(auditQ, entityType, entityID, fieldName, oldValue, oldIsEncrypted, actorID, reason); err != nil {
		return fmt.Errorf("repository: DeleteCustomField: write audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repository: DeleteCustomField: commit: %w", err)
	}
	return nil
}

// GetCustomFieldAuditLog returns all audit entries for the given entity's custom
// fields, ordered by most-recent first.
func (r *AdminRepository) GetCustomFieldAuditLog(entityType string, entityID int64) ([]models.EntityCustomFieldAudit, error) {
	const q = `
		SELECT id, entity_type, entity_id, field_name,
		       old_value, new_value, is_encrypted, actor_id, reason, changed_at
		FROM   entity_custom_fields_audit
		WHERE  entity_type = ? AND entity_id = ?
		ORDER  BY changed_at DESC`

	rows, err := r.db.Query(q, entityType, entityID)
	if err != nil {
		return nil, fmt.Errorf("repository: GetCustomFieldAuditLog: %w", err)
	}
	defer rows.Close()

	var out []models.EntityCustomFieldAudit
	for rows.Next() {
		var a models.EntityCustomFieldAudit
		if err := rows.Scan(
			&a.ID, &a.EntityType, &a.EntityID, &a.FieldName,
			&a.OldValue, &a.NewValue, &a.IsEncrypted, &a.ActorID, &a.Reason, &a.ChangedAt,
		); err != nil {
			return nil, fmt.Errorf("repository: GetCustomFieldAuditLog scan: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetEntityDisplayName returns a human-readable label for any supported entity
// type by querying the authoritative name column for that table.
//
//   entity_type   table       column
//   -----------   ---------   ------
//   user          users       username
//   material      materials   title
//   course        courses     name
//   location      locations   name
//
// Returns a safe fallback ("<type> #<id>") when the row is not found or the
// type is not one of the four above, so callers never need to handle an error.
func (r *AdminRepository) GetEntityDisplayName(entityType string, entityID int64) string {
	var q string
	switch entityType {
	case "user":
		q = `SELECT username FROM users WHERE id = ? AND deleted_at IS NULL`
	case "material":
		q = `SELECT title FROM materials WHERE id = ? AND deleted_at IS NULL`
	case "course":
		q = `SELECT name FROM courses WHERE id = ?`
	case "location":
		q = `SELECT name FROM locations WHERE id = ?`
	default:
		return entityType + " #" + strconv.FormatInt(entityID, 10)
	}
	var name string
	if err := r.db.QueryRow(q, entityID).Scan(&name); err != nil {
		return entityType + " #" + strconv.FormatInt(entityID, 10)
	}
	return name
}

// ---------------------------------------------------------------
// Duplicate detection
// ---------------------------------------------------------------

// DuplicatePair represents two users that may be the same person.
type DuplicatePair struct {
	UserA models.User
	UserB models.User
	Score float64 // similarity 0-1
}

// FindDuplicateUsers returns user pairs that are likely the same person.
//
// # Semantic mapping — schema fields → specification concepts
//
// The prompt specifies two signals: "exact ID" and "fuzzy(name, DOB)".
//
//   Spec concept          → Schema column     Rationale
//   ─────────────────────────────────────────────────────────────────────
//   Exact identifier (ID) → external_id       Dedicated column for the
//                                             institution-issued identifier
//                                             (student number, employee ID, etc.).
//                                             Two accounts sharing the same
//                                             non-null external_id are
//                                             definitively the same person.
//   Name (fuzzy)          → full_name         Dedicated real-name column.
//                                             Falls back to username when
//                                             full_name has not been set.
//   Date of birth (fuzzy) → date_of_birth     Direct column, exact match only
//                                             (birth dates are discrete values).
//
// # Detection strategies
//
//  1. Exact identifier match — both accounts share the same non-null external_id
//     → score 1.0.
//
//  2. Fuzzy identity match — composite weighted score for pairs NOT already
//     matched by Pass 1:
//
//       score = 0.6 × name_similarity  +  0.4 × dob_match
//
//     name_similarity: Levenshtein-based 0–1 ratio on full_name (username fallback).
//     dob_match:       1.0 when both records carry the same non-null DOB; 0 otherwise.
//
//     Threshold = 0.65 — ensures neither signal alone is sufficient:
//       - DOB match only  (0.4)     → below threshold, excluded.
//       - Name match only (max 0.6) → only identical names reach threshold;
//         any divergence requires a DOB match to compensate.
//       - Both signals    (≥ 0.65)  → included as probable duplicate.
//
// SQL pre-filters candidates to pairs sharing the same DOB or the same 4-char
// name prefix; Go then applies the full composite formula.
// Soft-deleted users are excluded. Results sorted by score desc, capped at limit.
func (r *AdminRepository) FindDuplicateUsers(limit int) ([]DuplicatePair, error) {
	// ---- Pass 1: exact ID — both accounts share the same non-null external_id_idx ----
	// Uses the HMAC blind index instead of the AES-GCM ciphertext.  Randomized
	// GCM encryption produces a unique ciphertext on every call, so equality
	// comparison on the raw external_id column would never match.  The blind index
	// is deterministic: HMAC-SHA256(derived_key, plaintext), so SQL = works.
	const exactQ = `
		SELECT a.id, a.username, a.email, a.password_hash, a.role,
		       a.failed_attempts, a.locked_until, a.date_of_birth,
		       a.full_name, a.full_name_idx, a.full_name_phonetic,
		       a.external_id, a.external_id_idx,
		       a.created_at, a.updated_at, a.deleted_at, a.must_change_password,
		       b.id, b.username, b.email, b.password_hash, b.role,
		       b.failed_attempts, b.locked_until, b.date_of_birth,
		       b.full_name, b.full_name_idx, b.full_name_phonetic,
		       b.external_id, b.external_id_idx,
		       b.created_at, b.updated_at, b.deleted_at, b.must_change_password
		FROM   users a
		JOIN   users b ON b.id > a.id
		             AND a.external_id_idx IS NOT NULL
		             AND a.external_id_idx = b.external_id_idx
		WHERE  a.deleted_at IS NULL AND b.deleted_at IS NULL
		ORDER  BY a.id`

	exactRows, err := r.db.Query(exactQ)
	if err != nil {
		return nil, fmt.Errorf("repository: FindDuplicateUsers (exact): %w", err)
	}
	pairs, err := scanUserPairs(exactRows)
	if err != nil {
		return nil, fmt.Errorf("repository: FindDuplicateUsers (exact scan): %w", err)
	}
	for i := range pairs {
		pairs[i].Score = 1.0
	}

	// ---- Pass 2: fuzzy — DOB exact + name/phonetic blind-index equality ----
	// Excludes pairs already matched in Pass 1 (same external_id_idx).
	// SQL pre-filter uses three deterministic signals:
	//   a) date_of_birth exact match,
	//   b) full_name_idx equality (HMAC — same plaintext → same index), OR
	//   c) full_name_phonetic equality (Soundex — similar-sounding names).
	// Go-level scoring applies the composite formula.
	const fuzzyQ = `
		SELECT a.id, a.username, a.email, a.password_hash, a.role,
		       a.failed_attempts, a.locked_until, a.date_of_birth,
		       a.full_name, a.full_name_idx, a.full_name_phonetic,
		       a.external_id, a.external_id_idx,
		       a.created_at, a.updated_at, a.deleted_at, a.must_change_password,
		       b.id, b.username, b.email, b.password_hash, b.role,
		       b.failed_attempts, b.locked_until, b.date_of_birth,
		       b.full_name, b.full_name_idx, b.full_name_phonetic,
		       b.external_id, b.external_id_idx,
		       b.created_at, b.updated_at, b.deleted_at, b.must_change_password
		FROM   users a
		JOIN   users b ON b.id > a.id
		             AND (a.external_id_idx IS NULL OR b.external_id_idx IS NULL
		                  OR a.external_id_idx != b.external_id_idx)
		WHERE  a.deleted_at IS NULL
		  AND  b.deleted_at IS NULL
		  AND (
		        (a.date_of_birth IS NOT NULL AND a.date_of_birth = b.date_of_birth)
		        OR (a.full_name_idx IS NOT NULL AND a.full_name_idx = b.full_name_idx)
		        OR (a.full_name_phonetic IS NOT NULL AND a.full_name_phonetic = b.full_name_phonetic)
		      )
		ORDER  BY a.id`

	fuzzyRows, err := r.db.Query(fuzzyQ)
	if err != nil {
		return nil, fmt.Errorf("repository: FindDuplicateUsers (fuzzy): %w", err)
	}
	candidates, err := scanUserPairs(fuzzyRows)
	if err != nil {
		return nil, fmt.Errorf("repository: FindDuplicateUsers (fuzzy scan): %w", err)
	}

	// Score each candidate:
	//   nameSim: 1.0  — blind-index match (identical plaintext)
	//            0.8  — phonetic (Soundex) match (similar-sounding names)
	//            Levenshtein on username as a fallback (no full_name on either account)
	//   dobMatch: 1.0 when both rows carry the same non-null date_of_birth.
	const (
		nameWeight = 0.6
		dobWeight  = 0.4
		minScore   = 0.65
	)
	for _, p := range candidates {
		var nameSim float64
		if p.UserA.FullNameIdx != nil && p.UserB.FullNameIdx != nil &&
			*p.UserA.FullNameIdx != "" && *p.UserA.FullNameIdx == *p.UserB.FullNameIdx {
			// Deterministic HMAC equality → plaintext names are identical.
			nameSim = 1.0
		} else if p.UserA.FullNamePhonetic != nil && p.UserB.FullNamePhonetic != nil &&
			*p.UserA.FullNamePhonetic != "" && *p.UserA.FullNamePhonetic == *p.UserB.FullNamePhonetic {
			// Same Soundex code → similar-sounding names (e.g. "Smith" / "Smyth").
			nameSim = 0.8
		} else if p.UserA.FullName == nil && p.UserB.FullName == nil {
			// Neither account has a full_name: fall back to username similarity.
			nameSim = usernameSimilarity(p.UserA.Username, p.UserB.Username)
		}
		// dob_match: 1.0 when both users share an identical, non-null birth date.
		var dobMatch float64
		if p.UserA.DateOfBirth != nil && p.UserB.DateOfBirth != nil &&
			*p.UserA.DateOfBirth == *p.UserB.DateOfBirth {
			dobMatch = 1.0
		}
		score := nameWeight*nameSim + dobWeight*dobMatch
		if score >= minScore {
			p.Score = score
			pairs = append(pairs, p)
		}
	}

	// Sort by score descending, apply limit.
	sortDuplicatePairs(pairs)
	if limit > 0 && len(pairs) > limit {
		pairs = pairs[:limit]
	}
	return pairs, nil
}

// ---------------------------------------------------------------
// Duplicate-detection helpers
// ---------------------------------------------------------------

// scanUserPairs scans rows produced by the self-join queries in FindDuplicateUsers.
// Column order must match the SELECT lists in exactQ and fuzzyQ exactly.
func scanUserPairs(rows *sql.Rows) ([]DuplicatePair, error) {
	defer rows.Close()
	var pairs []DuplicatePair
	for rows.Next() {
		var p DuplicatePair
		a, b := &p.UserA, &p.UserB
		if err := rows.Scan(
			&a.ID, &a.Username, &a.Email, &a.PasswordHash, &a.Role,
			&a.FailedAttempts, &a.LockedUntil, &a.DateOfBirth,
			&a.FullName, &a.FullNameIdx, &a.FullNamePhonetic,
			&a.ExternalID, &a.ExternalIDIdx,
			&a.CreatedAt, &a.UpdatedAt, &a.DeletedAt, &a.MustChangePassword,
			&b.ID, &b.Username, &b.Email, &b.PasswordHash, &b.Role,
			&b.FailedAttempts, &b.LockedUntil, &b.DateOfBirth,
			&b.FullName, &b.FullNameIdx, &b.FullNamePhonetic,
			&b.ExternalID, &b.ExternalIDIdx,
			&b.CreatedAt, &b.UpdatedAt, &b.DeletedAt, &b.MustChangePassword,
		); err != nil {
			return nil, err
		}
		pairs = append(pairs, p)
	}
	return pairs, rows.Err()
}

// usernameSimilarity returns a 0–1 Levenshtein-based similarity score for two
// usernames (case-insensitive).
func usernameSimilarity(a, b string) float64 {
	a = strings.ToLower(a)
	b = strings.ToLower(b)
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	if maxLen == 0 {
		return 1.0
	}
	dist := levenshtein(a, b)
	return 1.0 - float64(dist)/float64(maxLen)
}

// levenshtein computes the edit distance between two strings using the
// standard dynamic-programming approach.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	// Use two rows of the DP table to save memory.
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[j] = minInt(
				curr[j-1]+1,        // insertion
				prev[j]+1,          // deletion
				prev[j-1]+cost,     // substitution
			)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func minInt(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// sortDuplicatePairs sorts pairs by Score descending (insertion sort — small N).
func sortDuplicatePairs(pairs []DuplicatePair) {
	for i := 1; i < len(pairs); i++ {
		key := pairs[i]
		j := i - 1
		for j >= 0 && pairs[j].Score < key.Score {
			pairs[j+1] = pairs[j]
			j--
		}
		pairs[j+1] = key
	}
}

// MergeUsers merges duplicateID into primaryID:
//  1. Re-parents orders, ratings, comments, favorites_lists, notifications.
//  2. Inserts an entity_duplicates record.
//  3. Soft-deletes the duplicate user.
func (r *AdminRepository) MergeUsers(primaryID, duplicateID int64, mergedBy int64) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("repository: MergeUsers: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// 1a. Re-parent orders.
	if _, err := tx.Exec(`UPDATE orders SET user_id = ? WHERE user_id = ?`, primaryID, duplicateID); err != nil {
		return fmt.Errorf("repository: MergeUsers: orders: %w", err)
	}

	// 1b. Re-parent ratings (ignore conflicts — primary already has a rating).
	if _, err := tx.Exec(`
		UPDATE OR IGNORE ratings SET user_id = ? WHERE user_id = ?`,
		primaryID, duplicateID); err != nil {
		return fmt.Errorf("repository: MergeUsers: ratings: %w", err)
	}

	// 1c. Re-parent comments.
	if _, err := tx.Exec(`UPDATE comments SET user_id = ? WHERE user_id = ?`, primaryID, duplicateID); err != nil {
		return fmt.Errorf("repository: MergeUsers: comments: %w", err)
	}

	// 1d. Re-parent favorites_lists.
	if _, err := tx.Exec(`UPDATE favorites_lists SET user_id = ? WHERE user_id = ?`, primaryID, duplicateID); err != nil {
		return fmt.Errorf("repository: MergeUsers: favorites_lists: %w", err)
	}

	// 1e. Re-parent notifications.
	if _, err := tx.Exec(`UPDATE notifications SET user_id = ? WHERE user_id = ?`, primaryID, duplicateID); err != nil {
		return fmt.Errorf("repository: MergeUsers: notifications: %w", err)
	}

	// 2. Insert entity_duplicates record.
	const insertDupQ = `
		INSERT INTO entity_duplicates
		            (entity_type, primary_id, duplicate_id, status, merged_by, merged_at)
		VALUES      ('user', ?, ?, 'merged', ?, datetime('now'))`
	if _, err := tx.Exec(insertDupQ, primaryID, duplicateID, mergedBy); err != nil {
		return fmt.Errorf("repository: MergeUsers: insert entity_duplicates: %w", err)
	}

	// 3. Soft-delete the duplicate.
	const softDeleteQ = `
		UPDATE users
		SET    deleted_at = datetime('now'),
		       updated_at = datetime('now')
		WHERE  id = ? AND deleted_at IS NULL`
	if _, err := tx.Exec(softDeleteQ, duplicateID); err != nil {
		return fmt.Errorf("repository: MergeUsers: soft-delete duplicate: %w", err)
	}

	return tx.Commit()
}

// GetMergeHistory returns the most-recent limit entity_duplicates records.
func (r *AdminRepository) GetMergeHistory(limit int) ([]models.EntityDuplicate, error) {
	const q = `
		SELECT id, entity_type, primary_id, duplicate_id, status, merged_by, merged_at
		FROM   entity_duplicates
		ORDER  BY merged_at DESC
		LIMIT  ?`

	rows, err := r.db.Query(q, limit)
	if err != nil {
		return nil, fmt.Errorf("repository: GetMergeHistory: %w", err)
	}
	defer rows.Close()

	var out []models.EntityDuplicate
	for rows.Next() {
		var e models.EntityDuplicate
		if err := rows.Scan(
			&e.ID, &e.EntityType, &e.PrimaryID, &e.DuplicateID,
			&e.Status, &e.MergedBy, &e.MergedAt,
		); err != nil {
			return nil, fmt.Errorf("repository: GetMergeHistory: scan: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------
// Audit log
// ---------------------------------------------------------------

// WriteAuditLog inserts a row into the audit_log table.
func (r *AdminRepository) WriteAuditLog(actorID int64, action, entityType string, entityID int64, before, after interface{}, ip string) error {
	var beforeJSON, afterJSON *string
	if before != nil {
		b, err := json.Marshal(before)
		if err == nil {
			s := string(b)
			beforeJSON = &s
		}
	}
	if after != nil {
		b, err := json.Marshal(after)
		if err == nil {
			s := string(b)
			afterJSON = &s
		}
	}
	var ipPtr *string
	if ip != "" {
		ipPtr = &ip
	}

	const q = `
		INSERT INTO audit_log (actor_id, action, entity_type, entity_id, before_data, after_data, ip, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))`
	_, err := r.db.Exec(q, actorID, action, entityType, entityID, beforeJSON, afterJSON, ipPtr)
	if err != nil {
		return fmt.Errorf("repository: WriteAuditLog: %w", err)
	}
	return nil
}

// GetAuditLog returns paginated audit log entries for a specific entity.
func (r *AdminRepository) GetAuditLog(entityType string, entityID int64, limit, offset int) ([]models.AuditLog, error) {
	where := []string{"1=1"}
	args := []interface{}{}
	if entityType != "" {
		where = append(where, "entity_type = ?")
		args = append(args, entityType)
	}
	if entityID > 0 {
		where = append(where, "entity_id = ?")
		args = append(args, entityID)
	}
	q := `
		SELECT id, actor_id, action, entity_type, entity_id, before_data, after_data, ip, created_at
		FROM   audit_log
		WHERE  ` + strings.Join(where, " AND ") + `
		ORDER  BY created_at DESC
		LIMIT  ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("repository: GetAuditLog: %w", err)
	}
	defer rows.Close()
	return scanAuditLogs(rows)
}

// GetRecentAuditLog returns the most-recent limit audit log entries.
func (r *AdminRepository) GetRecentAuditLog(limit int) ([]models.AuditLog, error) {
	const q = `
		SELECT id, actor_id, action, entity_type, entity_id, before_data, after_data, ip, created_at
		FROM   audit_log
		ORDER  BY created_at DESC
		LIMIT  ?`

	rows, err := r.db.Query(q, limit)
	if err != nil {
		return nil, fmt.Errorf("repository: GetRecentAuditLog: %w", err)
	}
	defer rows.Close()
	return scanAuditLogs(rows)
}

// ---------------------------------------------------------------
// User management (admin)
// ---------------------------------------------------------------

// ListUsers returns paginated users, optionally filtered by role.
// The SELECT uses the canonical userCols order to match scanUserRow exactly.
func (r *AdminRepository) ListUsers(role string, limit, offset int) ([]models.User, error) {
	args := []interface{}{}
	where := "deleted_at IS NULL"
	if role != "" {
		where += " AND role = ?"
		args = append(args, role)
	}
	q := `SELECT ` + userCols + `
		FROM   users
		WHERE  ` + where + `
		ORDER  BY id
		LIMIT  ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("repository: ListUsers: %w", err)
	}
	defer rows.Close()

	var out []models.User
	for rows.Next() {
		u, err := scanUserRow(rows)
		if err != nil {
			return nil, fmt.Errorf("repository: ListUsers: scan: %w", err)
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

// UpdateUserRole changes the role of the given user.
func (r *AdminRepository) UpdateUserRole(userID int64, role string) error {
	const q = `UPDATE users SET role = ?, updated_at = datetime('now') WHERE id = ? AND deleted_at IS NULL`
	_, err := r.db.Exec(q, role, userID)
	if err != nil {
		return fmt.Errorf("repository: UpdateUserRole: %w", err)
	}
	return nil
}

// UnlockUser clears locked_until and resets failed_attempts.
func (r *AdminRepository) UnlockUser(userID int64) error {
	const q = `
		UPDATE users
		SET    locked_until    = NULL,
		       failed_attempts = 0,
		       updated_at      = datetime('now')
		WHERE  id = ? AND deleted_at IS NULL`
	_, err := r.db.Exec(q, userID)
	if err != nil {
		return fmt.Errorf("repository: UnlockUser: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------
// helpers
// ---------------------------------------------------------------

func scanAuditLogs(rows *sql.Rows) ([]models.AuditLog, error) {
	var out []models.AuditLog
	for rows.Next() {
		var a models.AuditLog
		if err := rows.Scan(
			&a.ID, &a.ActorID, &a.Action, &a.EntityType, &a.EntityID,
			&a.BeforeData, &a.AfterData, &a.IP, &a.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanAuditLogs: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
