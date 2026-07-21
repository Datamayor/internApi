package internships

import (
	"database/sql"
	"encoding/json"
	"intern-api/internal/middleware"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

type Handler struct {
	DB *sqlx.DB
}

type Internship struct {
	ID          int       `db:"id" json:"id"`
	Title       string    `db:"title" json:"title"`
	Description string    `db:"description" json:"description"`
	Department  string    `db:"department" json:"department"`
	Duration    string    `db:"duration" json:"duration"`
	Location    string    `db:"location" json:"location"`
	IsOpen      bool      `db:"is_open" json:"is_open"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

// GET /api/internships — public
func (h *Handler) GetAll(w http.ResponseWriter, r *http.Request) {
	var internships []Internship
	if err := h.DB.Select(&internships, `SELECT * FROM internships ORDER BY created_at DESC`); err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to fetch internships")
		return
	}
	middleware.JSON(w, http.StatusOK, internships)
}

// GET /api/internships/:id — public
func (h *Handler) GetOne(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var internship Internship
	err := h.DB.QueryRowx(`SELECT * FROM internships WHERE id = $1`, id).StructScan(&internship)
	if err == sql.ErrNoRows {
		middleware.Error(w, http.StatusNotFound, "internship not found")
		return
	} else if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "database error")
		return
	}

	middleware.JSON(w, http.StatusOK, internship)
}

// POST /api/internships — admin only
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Department  string `json:"department"`
		Duration    string `json:"duration"`
		Location    string `json:"location"`
		IsOpen      bool   `json:"is_open"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.Title == "" {
		middleware.Error(w, http.StatusBadRequest, "title is required")
		return
	}

	var internship Internship
	err := h.DB.QueryRowx(`
		INSERT INTO internships (title, description, department, duration, location, is_open)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING *
	`, body.Title, body.Description, body.Department, body.Duration, body.Location, body.IsOpen).StructScan(&internship)

	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to create internship")
		return
	}

	middleware.JSON(w, http.StatusCreated, internship)
}
