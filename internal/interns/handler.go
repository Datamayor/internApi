package interns

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

type Intern struct {
	ID           int        `db:"id" json:"id"`
	UserID       int        `db:"user_id" json:"user_id"`
	DepartmentID *int       `db:"department_id" json:"department_id"`
	SupervisorID *int       `db:"supervisor_id" json:"supervisor_id"`
	StartDate    *time.Time `db:"start_date" json:"start_date"`
	EndDate      *time.Time `db:"end_date" json:"end_date"`
	Status       string     `db:"status" json:"status"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time  `db:"updated_at" json:"updated_at"`
	Name         string     `db:"name" json:"name"`
	Email        string     `db:"email" json:"email"`
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
		middleware.Error(w, http.StatusInternalServerError, "failed to fetch interns")
		return
	}
	if interns == nil {
		interns = []Intern{}
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
		StartDate    string `json:"start_date"`
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
	`, body.UserID, body.DepartmentID, body.SupervisorID,
		nullString(body.StartDate), nullString(body.EndDate), body.Status,
	).StructScan(&intern)

	if err != nil {
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
	`, body.DepartmentID, body.SupervisorID,
		nullString(body.StartDate), nullString(body.EndDate), body.Status, id)

	if err != nil {
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


// GET /api/interns/status
func (h *Handler) GetByStatus(w http.ResponseWriter, r *http.Request) {
	statusFilter := r.URL.Query().Get("status")

	var interns []Intern
	err := h.DB.Select(&interns, `
		SELECT i.*, u.name, u.email
		FROM interns i
		JOIN users u ON u.id = i.user_id
		ORDER BY i.status, u.name
	`)
	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to fetch interns")
		return
	}

	// leave_requests.intern_id actually references users.id, not interns.id
	onLeaveUserIDs := map[int]bool{}
	var userIDs []int
	err = h.DB.Select(&userIDs, `
		SELECT intern_id FROM leave_requests
		WHERE status = 'approved'
		AND start_date <= CURRENT_DATE
		AND end_date >= CURRENT_DATE
	`)
	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to fetch leave data")
		return
	}
	for _, uid := range userIDs {
		onLeaveUserIDs[uid] = true
	}

	// Initialize empty slices so JSON returns [] not null
	grouped := map[string][]Intern{
		"active":     {},
		"on_leave":   {},
		"completed":  {},
		"terminated": {},
	}

	for _, intern := range interns {
		effectiveStatus := intern.Status
		if effectiveStatus == "active" && onLeaveUserIDs[intern.UserID] {
			effectiveStatus = "on_leave"
		}
		if _, ok := grouped[effectiveStatus]; ok {
			grouped[effectiveStatus] = append(grouped[effectiveStatus], intern)
		}
	}

	// If a status filter was passed, narrow the response to just that bucket
	if statusFilter != "" {
		filtered := map[string][]Intern{
			"active":     {},
			"on_leave":   {},
			"completed":  {},
			"terminated": {},
		}
		if list, ok := grouped[statusFilter]; ok {
			filtered[statusFilter] = list
		}
		grouped = filtered
	}

	middleware.JSON(w, http.StatusOK, map[string]interface{}{
		"summary": map[string]int{
			"active":     len(grouped["active"]),
			"on_leave":   len(grouped["on_leave"]),
			"completed":  len(grouped["completed"]),
			"terminated": len(grouped["terminated"]),
			"total":      len(grouped["active"]) + len(grouped["on_leave"]) + len(grouped["completed"]) + len(grouped["terminated"]),
		},
		"interns": grouped,
	})
}

func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
