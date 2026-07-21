package supervisors

import (
	"database/sql"
	"encoding/json"
	"intern-api/internal/middleware"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

type Handler struct {
	DB *sqlx.DB
}

type Supervisor struct {
	ID           int       `db:"id" json:"id"`
	UserID       int       `db:"user_id" json:"user_id"`
	DepartmentID *int      `db:"department_id" json:"department_id"`
	Phone        string    `db:"phone" json:"phone"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time `db:"updated_at" json:"updated_at"`
	// Joined from users
	Name  string `db:"name" json:"name"`
	Email string `db:"email" json:"email"`
}

// GET /api/supervisors
func (h *Handler) GetAll(w http.ResponseWriter, r *http.Request) {
	var supervisors []Supervisor
	err := h.DB.Select(&supervisors, `
		SELECT s.*, u.name, u.email
		FROM supervisors s
		JOIN users u ON u.id = s.user_id
		ORDER BY u.name
	`)
	if err != nil {
		log.Println("GetAll supervisors db error:", err)
		middleware.Error(w, http.StatusInternalServerError, "failed to fetch supervisors")
		return
	}
	middleware.JSON(w, http.StatusOK, supervisors)
}

// GET /api/supervisors/:id
func (h *Handler) GetOne(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var s Supervisor
	err := h.DB.QueryRowx(`
		SELECT s.*, u.name, u.email
		FROM supervisors s
		JOIN users u ON u.id = s.user_id
		WHERE s.id = $1
	`, id).StructScan(&s)

	if err == sql.ErrNoRows {
		middleware.Error(w, http.StatusNotFound, "supervisor not found")
		return
	} else if err != nil {
		log.Println("GetOne supervisors db error:", err)
		middleware.Error(w, http.StatusInternalServerError, "database error")
		return
	}

	middleware.JSON(w, http.StatusOK, s)
}

// POST /api/supervisors
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID       int    `json:"user_id"`
		DepartmentID *int   `json:"department_id"`
		Phone        string `json:"phone"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.UserID == 0 {
		middleware.Error(w, http.StatusBadRequest, "user_id is required")
		return
	}

	var s Supervisor
	err := h.DB.QueryRowx(
		`INSERT INTO supervisors (user_id, department_id, phone) VALUES ($1, $2, $3) RETURNING *`,
		body.UserID, body.DepartmentID, body.Phone,
	).StructScan(&s)

	if err != nil {
		log.Println("Create supervisor db error:", err)
		middleware.Error(w, http.StatusInternalServerError, "failed to create supervisor")
		return
	}

	middleware.JSON(w, http.StatusCreated, s)
}

// PUT /api/supervisors/:id
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var body struct {
		DepartmentID *int   `json:"department_id"`
		Phone        string `json:"phone"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := h.DB.Exec(
		`UPDATE supervisors SET department_id = $1, phone = $2, updated_at = NOW() WHERE id = $3`,
		body.DepartmentID, body.Phone, id,
	)
	if err != nil {
		log.Println("Update supervisor db error:", err)
		middleware.Error(w, http.StatusInternalServerError, "failed to update supervisor")
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		middleware.Error(w, http.StatusNotFound, "supervisor not found")
		return
	}

	middleware.JSON(w, http.StatusOK, map[string]string{"message": "supervisor updated"})
}

// DELETE /api/supervisors/:id
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	result, err := h.DB.Exec(`DELETE FROM supervisors WHERE id = $1`, id)
	if err != nil {
		log.Println("Delete supervisor db error:", err)
		middleware.Error(w, http.StatusInternalServerError, "failed to delete supervisor")
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		middleware.Error(w, http.StatusNotFound, "supervisor not found")
		return
	}

	middleware.JSON(w, http.StatusOK, map[string]string{"message": "supervisor deleted"})
}