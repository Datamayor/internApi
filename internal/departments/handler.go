package departments

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

type Department struct {
	ID          int       `db:"id" json:"id"`
	Name        string    `db:"name" json:"name"`
	Description string    `db:"description" json:"description"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

// GET /api/departments
func (h *Handler) GetAll(w http.ResponseWriter, r *http.Request) {
	departments := []Department{}
	if err := h.DB.Select(&departments, `SELECT * FROM departments ORDER BY name`); err != nil {
		log.Println("GetAll departments db error:", err)
		middleware.Error(w, http.StatusInternalServerError, "failed to fetch departments")
		return
	}
	middleware.JSON(w, http.StatusOK, departments)
}

// POST /api/departments
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.Name == "" {
		middleware.Error(w, http.StatusBadRequest, "name is required")
		return
	}

	var dept Department
	err := h.DB.QueryRowx(
		`INSERT INTO departments (name, description) VALUES ($1, $2) RETURNING *`,
		body.Name, body.Description,
	).StructScan(&dept)

	if err != nil {
		log.Println("Create department db error:", err)
		middleware.Error(w, http.StatusInternalServerError, "failed to create department")
		return
	}

	middleware.JSON(w, http.StatusCreated, dept)
}

// PUT /api/departments/:id
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := h.DB.Exec(
		`UPDATE departments SET name = $1, description = $2, updated_at = NOW() WHERE id = $3`,
		body.Name, body.Description, id,
	)
	if err != nil {
		log.Println("Update department db error:", err)
		middleware.Error(w, http.StatusInternalServerError, "failed to update department")
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		middleware.Error(w, http.StatusNotFound, "department not found")
		return
	}

	middleware.JSON(w, http.StatusOK, map[string]string{"message": "department updated"})
}

// DELETE /api/departments/:id
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	result, err := h.DB.Exec(`DELETE FROM departments WHERE id = $1`, id)
	if err != nil {
		log.Println("Delete department db error:", err)
		middleware.Error(w, http.StatusInternalServerError, "failed to delete department")
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		middleware.Error(w, http.StatusNotFound, "department not found")
		return
	}

	middleware.JSON(w, http.StatusOK, map[string]string{"message": "department deleted"})
}

// GET /api/departments/:id — get single department
func (h *Handler) GetOne(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var dept Department
	err := h.DB.QueryRowx(`SELECT * FROM departments WHERE id = $1`, id).StructScan(&dept)
	if err == sql.ErrNoRows {
		middleware.Error(w, http.StatusNotFound, "department not found")
		return
	} else if err != nil {
		log.Println("GetOne department db error:", err)
		middleware.Error(w, http.StatusInternalServerError, "database error")
		return
	}

	middleware.JSON(w, http.StatusOK, dept)
}