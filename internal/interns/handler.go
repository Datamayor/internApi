package interns

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

type Intern struct {
	ID           int        `db:"id" json:"id"`
	UserID       int        `db:"user_id" json:"user_id"`
	DepartmentID *int       `db:"department_id" json:"department_id"`
	SupervisorID *int       `db:"supervisor_id" json:"supervisor_id"`
	StartDate    *time.Time `db:"start_date" json:"start_date"`
	EndDate      *time.Time `db:"end_date" json:"end_date"`
	Status       string     `db:"status" json:"status"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time  `db:"updated_at" json:"updated_at"` // <-- add this
	// Joined fields from users table
	Name  string `db:"name" json:"name"`
	Email string `db:"email" json:"email"`
}

// GET /api/interns
func (h *Handler) GetAll(w http.ResponseWriter, r *http.Request) {
	var interns []Intern
	err := h.DB.Select(&interns, `
		SELECT i.*, u.name, u.email
		FROM interns i
		JOIN users u ON u.id = i.user_id
		ORDER BY i.created_at DESC
	`)
	if err != nil {
		log.Println("GetAll db error:", err)
		middleware.Error(w, http.StatusInternalServerError, "failed to fetch interns")
		return
	}
	middleware.JSON(w, http.StatusOK, interns)
}

// GET /api/interns/:id
func (h *Handler) GetOne(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var intern Intern
	err := h.DB.QueryRowx(`
		SELECT i.*, u.name, u.email
		FROM interns i
		JOIN users u ON u.id = i.user_id
		WHERE i.id = $1
	`, id).StructScan(&intern)

	if err == sql.ErrNoRows {
		middleware.Error(w, http.StatusNotFound, "intern not found")
		return
	} else if err != nil {
		log.Println("GetOne db error:", err)
		middleware.Error(w, http.StatusInternalServerError, "database error")
		return
	}

	middleware.JSON(w, http.StatusOK, intern)
}

// POST /api/interns
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID       int    `json:"user_id"`
		DepartmentID *int   `json:"department_id"`
		SupervisorID *int   `json:"supervisor_id"`
		StartDate    string `json:"start_date"` // "YYYY-MM-DD"
		EndDate      string `json:"end_date"`
		Status       string `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.UserID == 0 {
		middleware.Error(w, http.StatusBadRequest, "user_id is required")
		return
	}

	if body.Status == "" {
		body.Status = "active"
	}

	var intern Intern
	err := h.DB.QueryRowx(`
    INSERT INTO interns (user_id, department_id, supervisor_id, start_date, end_date, status)
    VALUES ($1, $2, $3, $4, $5, $6)
    RETURNING id, user_id, department_id, supervisor_id, start_date, end_date, status, created_at
`, body.UserID, body.DepartmentID, body.SupervisorID, nullString(body.StartDate), nullString(body.EndDate), body.Status,
	).StructScan(&intern)

	if err != nil {
		log.Println("Create db error:", err)
		middleware.Error(w, http.StatusInternalServerError, "failed to create intern")
		return
	}

	middleware.JSON(w, http.StatusCreated, intern)
}

// PUT /api/interns/:id
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var body struct {
		DepartmentID *int   `json:"department_id"`
		SupervisorID *int   `json:"supervisor_id"`
		StartDate    string `json:"start_date"`
		EndDate      string `json:"end_date"`
		Status       string `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	_, err := h.DB.Exec(`
		UPDATE interns
		SET department_id = $1, supervisor_id = $2, start_date = $3, end_date = $4, status = $5
		WHERE id = $6
	`, body.DepartmentID, body.SupervisorID, nullString(body.StartDate), nullString(body.EndDate), body.Status, id)

	if err != nil {
		log.Println("Update db error:", err)
		middleware.Error(w, http.StatusInternalServerError, "failed to update intern")
		return
	}

	middleware.JSON(w, http.StatusOK, map[string]string{"message": "intern updated"})
}

// DELETE /api/interns/:id
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	result, err := h.DB.Exec(`DELETE FROM interns WHERE id = $1`, id)
	if err != nil {
		log.Println("Delete db error:", err)
		middleware.Error(w, http.StatusInternalServerError, "failed to delete intern")
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		middleware.Error(w, http.StatusNotFound, "intern not found")
		return
	}

	middleware.JSON(w, http.StatusOK, map[string]string{"message": "intern deleted"})
}

// nullString returns nil if s is empty (so SQL treats it as NULL)
func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
