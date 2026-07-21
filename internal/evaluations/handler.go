package evaluations

import (
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

type Evaluation struct {
	ID           int       `db:"id" json:"id"`
	InternID     int       `db:"intern_id" json:"intern_id"`
	SupervisorID *int      `db:"supervisor_id" json:"supervisor_id"`
	Score        int       `db:"score" json:"score"`
	Comments     string    `db:"comments" json:"comments"`
	Period       string    `db:"period" json:"period"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time `db:"updated_at" json:"updated_at"`
}

// POST /api/evaluations
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		InternID     int    `json:"intern_id"`
		SupervisorID *int   `json:"supervisor_id"`
		Score        int    `json:"score"`
		Comments     string `json:"comments"`
		Period       string `json:"period"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.InternID == 0 {
		middleware.Error(w, http.StatusBadRequest, "intern_id is required")
		return
	}

	if body.Score < 1 || body.Score > 10 {
		middleware.Error(w, http.StatusBadRequest, "score must be between 1 and 10")
		return
	}

	var eval Evaluation
	err := h.DB.QueryRowx(`
		INSERT INTO evaluations (intern_id, supervisor_id, score, comments, period)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING *
	`, body.InternID, body.SupervisorID, body.Score, body.Comments, body.Period).StructScan(&eval)

	if err != nil {
		log.Println("Create evaluation db error:", err)
		middleware.Error(w, http.StatusInternalServerError, "failed to create evaluation")
		return
	}

	middleware.JSON(w, http.StatusCreated, eval)
}

// GET /api/evaluations — all evaluations
func (h *Handler) GetAll(w http.ResponseWriter, r *http.Request) {
	evals := []Evaluation{}
	if err := h.DB.Select(&evals, `SELECT * FROM evaluations ORDER BY created_at DESC`); err != nil {
		log.Println("GetAll evaluations db error:", err)
		middleware.Error(w, http.StatusInternalServerError, "failed to fetch evaluations")
		return
	}
	middleware.JSON(w, http.StatusOK, evals)
}

// GET /api/evaluations/:internId — evaluations for one intern
func (h *Handler) GetByIntern(w http.ResponseWriter, r *http.Request) {
	internID := chi.URLParam(r, "internId")

	evals := []Evaluation{}
	if err := h.DB.Select(&evals,
		`SELECT * FROM evaluations WHERE intern_id = $1 ORDER BY created_at DESC`, internID,
	); err != nil {
		log.Println("GetByIntern evaluations db error:", err)
		middleware.Error(w, http.StatusInternalServerError, "failed to fetch evaluations")
		return
	}
	middleware.JSON(w, http.StatusOK, evals)
}

// PUT /api/evaluations/:id
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var body struct {
		Score    int    `json:"score"`
		Comments string `json:"comments"`
		Period   string `json:"period"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.Score != 0 && (body.Score < 1 || body.Score > 10) {
		middleware.Error(w, http.StatusBadRequest, "score must be between 1 and 10")
		return
	}

	result, err := h.DB.Exec(`
		UPDATE evaluations
		SET score = $1, comments = $2, period = $3, updated_at = NOW()
		WHERE id = $4
	`, body.Score, body.Comments, body.Period, id)

	if err != nil {
		log.Println("Update evaluation db error:", err)
		middleware.Error(w, http.StatusInternalServerError, "failed to update evaluation")
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		middleware.Error(w, http.StatusNotFound, "evaluation not found")
		return
	}

	middleware.JSON(w, http.StatusOK, map[string]string{"message": "evaluation updated"})
}