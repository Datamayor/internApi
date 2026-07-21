package attendance

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

type Attendance struct {
	ID        int        `db:"id" json:"id"`
	InternID  int        `db:"intern_id" json:"intern_id"`
	Date      time.Time  `db:"date" json:"date"`
	CheckIn   *time.Time `db:"check_in" json:"check_in"`
	CheckOut  *time.Time `db:"check_out" json:"check_out"`
	CreatedAt time.Time  `db:"created_at" json:"created_at"`
}

// GET /api/attendance — get all attendance records
func (h *Handler) GetAll(w http.ResponseWriter, r *http.Request) {
	var records []Attendance
	if err := h.DB.Select(&records, `SELECT * FROM attendance ORDER BY date DESC`); err != nil {
		log.Println("GetAll attendance db error:", err)
		middleware.Error(w, http.StatusInternalServerError, "failed to fetch attendance")
		return
	}
	middleware.JSON(w, http.StatusOK, records)
}

// GET /api/attendance/:internId — get attendance for a specific intern
func (h *Handler) GetByIntern(w http.ResponseWriter, r *http.Request) {
	internID := chi.URLParam(r, "internId")

	var records []Attendance
	if err := h.DB.Select(&records, `SELECT * FROM attendance WHERE intern_id = $1 ORDER BY date DESC`, internID); err != nil {
		log.Println("GetByIntern attendance db error:", err)
		middleware.Error(w, http.StatusInternalServerError, "failed to fetch attendance")
		return
	}
	middleware.JSON(w, http.StatusOK, records)
}

// POST /api/attendance/check-in
func (h *Handler) CheckIn(w http.ResponseWriter, r *http.Request) {
	var body struct {
		InternID int    `json:"intern_id"`
		Date     string `json:"date"` // "YYYY-MM-DD", defaults to today if empty
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.InternID == 0 {
		middleware.Error(w, http.StatusBadRequest, "intern_id is required")
		return
	}

	date := body.Date
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	// Check if a record already exists for this intern today
	var existingID int
	err := h.DB.QueryRow(
		`SELECT id FROM attendance WHERE intern_id = $1 AND date = $2`,
		body.InternID, date,
	).Scan(&existingID)

	if err == nil {
		// Record exists — update check_in
		_, err = h.DB.Exec(
			`UPDATE attendance SET check_in = NOW() WHERE id = $1`,
			existingID,
		)
	} else if err == sql.ErrNoRows {
		// No record yet — create one
		_, err = h.DB.Exec(
			`INSERT INTO attendance (intern_id, date, check_in) VALUES ($1, $2, NOW())`,
			body.InternID, date,
		)
	}
	// else: err is some other DB error (e.g. connection issue) — fall through and report it below

	if err != nil {
		log.Println("CheckIn db error:", err)
		middleware.Error(w, http.StatusInternalServerError, "failed to record check-in")
		return
	}

	middleware.JSON(w, http.StatusOK, map[string]string{"message": "check-in recorded"})
}

// POST /api/attendance/check-out
func (h *Handler) CheckOut(w http.ResponseWriter, r *http.Request) {
	var body struct {
		InternID int    `json:"intern_id"`
		Date     string `json:"date"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.InternID == 0 {
		middleware.Error(w, http.StatusBadRequest, "intern_id is required")
		return
	}

	date := body.Date
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	result, err := h.DB.Exec(
		`UPDATE attendance SET check_out = NOW() WHERE intern_id = $1 AND date = $2`,
		body.InternID, date,
	)
	if err != nil {
		log.Println("CheckOut db error:", err)
		middleware.Error(w, http.StatusInternalServerError, "failed to record check-out")
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		middleware.Error(w, http.StatusNotFound, "no check-in record found for today — check in first")
		return
	}

	middleware.JSON(w, http.StatusOK, map[string]string{"message": "check-out recorded"})
}