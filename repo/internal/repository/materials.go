package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"w2t86/internal/models"
)

// MaterialRepository provides database operations for the materials table.
type MaterialRepository struct {
	db *sql.DB
}

// NewMaterialRepository returns a MaterialRepository backed by the given database.
func NewMaterialRepository(db *sql.DB) *MaterialRepository {
	return &MaterialRepository{db: db}
}

// Create inserts a new material row and returns the populated model.
func (r *MaterialRepository) Create(m *models.Material) (*models.Material, error) {
	const q = `
		INSERT INTO materials (isbn, title, author, publisher, edition, subject, grade_level,
		                       total_qty, available_qty, reserved_qty, price, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id, isbn, title, author, publisher, edition, subject, grade_level,
		          total_qty, available_qty, reserved_qty, price, status, created_at, updated_at, deleted_at`

	row := r.db.QueryRow(q,
		m.ISBN, m.Title, m.Author, m.Publisher, m.Edition, m.Subject, m.GradeLevel,
		m.TotalQty, m.AvailableQty, m.ReservedQty, m.Price, m.Status,
	)
	return scanMaterial(row)
}

// GetByID returns the material with the given id (excluding soft-deleted rows).
func (r *MaterialRepository) GetByID(id int64) (*models.Material, error) {
	const q = `
		SELECT id, isbn, title, author, publisher, edition, subject, grade_level,
		       total_qty, available_qty, reserved_qty, price, status, created_at, updated_at, deleted_at
		FROM   materials
		WHERE  id = ? AND deleted_at IS NULL`

	row := r.db.QueryRow(q, id)
	return scanMaterial(row)
}

// Update applies the given field map to the material identified by id.
// Only columns present in the map are changed; updated_at is always refreshed.
func (r *MaterialRepository) Update(id int64, fields map[string]interface{}) error {
	if len(fields) == 0 {
		return nil
	}

	allowed := map[string]bool{
		"isbn":          true,
		"title":         true,
		"author":        true,
		"publisher":     true,
		"edition":       true,
		"subject":       true,
		"grade_level":   true,
		"total_qty":     true,
		"available_qty": true,
		"reserved_qty":  true,
		"price":         true,
		"status":        true,
	}

	setClauses := make([]string, 0, len(fields)+1)
	args := make([]interface{}, 0, len(fields)+2)

	for col, val := range fields {
		if !allowed[col] {
			return fmt.Errorf("repository: material Update: unknown or disallowed column %q", col)
		}
		setClauses = append(setClauses, col+" = ?")
		args = append(args, val)
	}
	setClauses = append(setClauses, "updated_at = datetime('now')")
	args = append(args, id)

	q := "UPDATE materials SET " + strings.Join(setClauses, ", ") + " WHERE id = ? AND deleted_at IS NULL"
	_, err := r.db.Exec(q, args...)
	return err
}

// SoftDelete sets deleted_at on the material row.
func (r *MaterialRepository) SoftDelete(id int64) error {
	const q = `UPDATE materials SET deleted_at = datetime('now'), updated_at = datetime('now') WHERE id = ? AND deleted_at IS NULL`
	_, err := r.db.Exec(q, id)
	return err
}

// List returns materials with optional filters (subject, grade_level, status).
func (r *MaterialRepository) List(limit, offset int, filters map[string]string) ([]models.Material, error) {
	allowed := map[string]bool{
		"subject":     true,
		"grade_level": true,
		"status":      true,
	}

	where := []string{"deleted_at IS NULL"}
	args := []interface{}{}

	for col, val := range filters {
		if allowed[col] && val != "" {
			where = append(where, col+" = ?")
			args = append(args, val)
		}
	}

	q := `SELECT id, isbn, title, author, publisher, edition, subject, grade_level,
		         total_qty, available_qty, reserved_qty, price, status, created_at, updated_at, deleted_at
		  FROM   materials
		  WHERE  ` + strings.Join(where, " AND ") + `
		  ORDER  BY id
		  LIMIT  ? OFFSET ?`

	args = append(args, limit, offset)

	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var materials []models.Material
	for rows.Next() {
		m, err := scanMaterialRow(rows)
		if err != nil {
			return nil, err
		}
		materials = append(materials, *m)
	}
	return materials, rows.Err()
}

// Search performs a full-text search using the materials_fts virtual table.
func (r *MaterialRepository) Search(query string, limit, offset int) ([]models.Material, error) {
	const q = `
		SELECT m.id, m.isbn, m.title, m.author, m.publisher, m.edition, m.subject, m.grade_level,
		       m.total_qty, m.available_qty, m.reserved_qty, m.price, m.status, m.created_at, m.updated_at, m.deleted_at
		FROM   materials_fts fts
		JOIN   materials m ON m.id = fts.rowid
		WHERE  materials_fts MATCH ? AND m.deleted_at IS NULL
		ORDER  BY rank
		LIMIT  ? OFFSET ?`

	rows, err := r.db.Query(q, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var materials []models.Material
	for rows.Next() {
		m, err := scanMaterialRow(rows)
		if err != nil {
			return nil, err
		}
		materials = append(materials, *m)
	}
	return materials, rows.Err()
}

// Reserve decrements available_qty and increments reserved_qty atomically.
// Returns an error if available_qty would go below zero.
func (r *MaterialRepository) Reserve(id int64, qty int) error {
	const q = `
		UPDATE materials
		SET    available_qty = available_qty - ?,
		       reserved_qty  = reserved_qty  + ?,
		       updated_at    = datetime('now')
		WHERE  id = ? AND deleted_at IS NULL AND available_qty >= ?`

	res, err := r.db.Exec(q, qty, qty, id, qty)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("repository: Reserve: insufficient available quantity for material %d", id)
	}
	return nil
}

// Release increments available_qty and decrements reserved_qty atomically.
// This is the correct operation for pre-fulfillment cancellations (reserved →
// available). It returns an error if reserved_qty would drop below zero,
// preventing invariant violations from incorrect post-fulfillment calls.
func (r *MaterialRepository) Release(id int64, qty int) error {
	const q = `
		UPDATE materials
		SET    available_qty = available_qty + ?,
		       reserved_qty  = reserved_qty  - ?,
		       updated_at    = datetime('now')
		WHERE  id = ? AND deleted_at IS NULL AND reserved_qty >= ?`

	res, err := r.db.Exec(q, qty, qty, id, qty)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("repository: Release: insufficient reserved quantity for material %d (would go negative)", id)
	}
	return nil
}

// ReturnToStock increments available_qty only. Use this for post-fulfillment
// returns where reserved_qty has already been decremented by the completion
// transition. Calling Release after fulfillment would drive reserved_qty
// negative; ReturnToStock is the correct post-fulfillment inventory operation.
func (r *MaterialRepository) ReturnToStock(id int64, qty int) error {
	const q = `
		UPDATE materials
		SET    available_qty = available_qty + ?,
		       updated_at    = datetime('now')
		WHERE  id = ? AND deleted_at IS NULL`

	_, err := r.db.Exec(q, qty, id)
	return err
}

// DirectIssue decrements available_qty only — used when a copy is handed out
// directly without a prior reservation step (e.g. the new leg of an exchange
// on a completed order where the item is issued immediately, not queued).
// Returns an error if available_qty would drop below zero.
func (r *MaterialRepository) DirectIssue(id int64, qty int) error {
	const q = `
		UPDATE materials
		SET    available_qty = available_qty - ?,
		       updated_at    = datetime('now')
		WHERE  id = ? AND deleted_at IS NULL AND available_qty >= ?`

	res, err := r.db.Exec(q, qty, id, qty)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("repository: DirectIssue: insufficient available quantity for material %d", id)
	}
	return nil
}

// Fulfill decrements reserved_qty when items are actually issued.
func (r *MaterialRepository) Fulfill(id int64, qty int) error {
	const q = `
		UPDATE materials
		SET    reserved_qty = reserved_qty - ?,
		       updated_at   = datetime('now')
		WHERE  id = ? AND deleted_at IS NULL`

	_, err := r.db.Exec(q, qty, id)
	return err
}

// DB returns the underlying *sql.DB so service-layer callers can pass it to
// WriteVersion without needing a separate dependency.
func (r *MaterialRepository) DB() *sql.DB {
	return r.db
}

// WriteVersion inserts a material_versions record. Intended to be called inside
// the same transaction as Create/Update so the history is always consistent.
func (r *MaterialRepository) WriteVersion(db *sql.DB, materialID int64, actorID int64, data interface{}) error {
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("repository: WriteVersion: marshal: %w", err)
	}
	const q = `
		INSERT INTO material_versions (material_id, changed_by, change_data, changed_at)
		VALUES (?, ?, ?, datetime('now'))`
	_, err = db.Exec(q, materialID, actorID, string(b))
	return err
}

// ---------------------------------------------------------------
// helpers
// ---------------------------------------------------------------

type materialScanner interface {
	Scan(dest ...interface{}) error
}

func scanMaterial(s materialScanner) (*models.Material, error) {
	m := &models.Material{}
	err := s.Scan(
		&m.ID, &m.ISBN, &m.Title, &m.Author, &m.Publisher, &m.Edition,
		&m.Subject, &m.GradeLevel,
		&m.TotalQty, &m.AvailableQty, &m.ReservedQty, &m.Price, &m.Status,
		&m.CreatedAt, &m.UpdatedAt, &m.DeletedAt,
	)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func scanMaterialRow(rows *sql.Rows) (*models.Material, error) {
	m := &models.Material{}
	err := rows.Scan(
		&m.ID, &m.ISBN, &m.Title, &m.Author, &m.Publisher, &m.Edition,
		&m.Subject, &m.GradeLevel,
		&m.TotalQty, &m.AvailableQty, &m.ReservedQty, &m.Price, &m.Status,
		&m.CreatedAt, &m.UpdatedAt, &m.DeletedAt,
	)
	if err != nil {
		return nil, err
	}
	return m, nil
}

