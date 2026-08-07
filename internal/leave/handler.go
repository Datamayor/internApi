package leave

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

type LeaveRequest struct {
	ID          int        `db:"id" json:"id"`
	InternID    int        `db:"intern_id" json:"intern_id"`
	Reason      string     `db:"reason" json:"reason"`
	StartDate   time.Time  `db:"start_date" json:"start_date"`
	EndDate     time.Time  `db:"end_date" json:"end_date"`
	Status      string     `db:"status" json:"status"` // pending, approved, rejected
	ReviewedBy  *int       `db:"reviewed_by" json:"reviewed_by"`
	ReviewNote  string     `db:"review_note" json:"review_note"`
	CreatedAt   time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at" json:"updated_at"`
	// Joined fields
	InternName  string     `db:"intern_name" json:"intern_name"`
	ReviewerName string    `db:"reviewer_name" json:"reviewer_name"`
}

// POST /api/leave — intern submits a leave request
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		InternID  int    `json:"intern_id"`
		Reason    string `json:"reason"`
		StartDate string `json:"start_date"` // YYYY-MM-DD
		EndDate   string `json:"end_date"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.InternID == 0 || body.Reason == "" || body.StartDate == "" || body.EndDate == "" {
		middleware.Error(w, http.StatusBadRequest, "intern_id, reason, start_date and end_date are required")
		return
	}

	var req LeaveRequest
	err := h.DB.QueryRowx(`
		INSERT INTO leave_requests (intern_id, reason, start_date, end_date, status)
		VALUES ($1, $2, $3, $4, 'pending')
		RETURNING *
	`, body.InternID, body.Reason, body.StartDate, body.EndDate).StructScan(&req)

	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to submit leave request")
		return
	}

	middleware.JSON(w, http.StatusCreated, req)
}

// GET /api/leave — get all leave requests (supervisor, hr)
func (h *Handler) GetAll(w http.ResponseWriter, r *http.Request) {
	// Optional filter: ?status=pending or ?status=approved
	status := r.URL.Query().Get("status")

	var requests []LeaveRequest
	var err error

	if status != "" {
		err = h.DB.Select(&requests, `
			SELECT lr.*,
				COALESCE(u.name, '') AS intern_name,
				COALESCE(r.name, '') AS reviewer_name
			FROM leave_requests lr
			LEFT JOIN users u ON u.id = lr.intern_id
			LEFT JOIN users r ON r.id = lr.reviewed_by
			WHERE lr.status = $1
			ORDER BY lr.created_at DESC
		`, status)
	} else {
		err = h.DB.Select(&requests, `
			SELECT lr.*,
				COALESCE(u.name, '') AS intern_name,
				COALESCE(r.name, '') AS reviewer_name
			FROM leave_requests lr
			LEFT JOIN users u ON u.id = lr.intern_id
			LEFT JOIN users r ON r.id = lr.reviewed_by
			ORDER BY lr.created_at DESC
		`)
	}

	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to fetch leave requests")
		return
	}

	middleware.JSON(w, http.StatusOK, requests)
}

// GET /api/leave/intern/:internId — get leave requests for a specific intern
func (h *Handler) GetByIntern(w http.ResponseWriter, r *http.Request) {
	internID := chi.URLParam(r, "internId")

	var requests []LeaveRequest
	err := h.DB.Select(&requests, `
		SELECT lr.*,
			COALESCE(u.name, '') AS intern_name,
			COALESCE(rv.name, '') AS reviewer_name
		FROM leave_requests lr
		LEFT JOIN users u ON u.id = lr.intern_id
		LEFT JOIN users rv ON rv.id = lr.reviewed_by
		WHERE lr.intern_id = $1
		ORDER BY lr.created_at DESC
	`, internID)

	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to fetch leave requests")
		return
	}

	middleware.JSON(w, http.StatusOK, requests)
}

// GET /api/leave/:id — get single leave request
func (h *Handler) GetOne(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req LeaveRequest
	err := h.DB.QueryRowx(`
		SELECT lr.*,
			COALESCE(u.name, '') AS intern_name,
			COALESCE(rv.name, '') AS reviewer_name
		FROM leave_requests lr
		LEFT JOIN users u ON u.id = lr.intern_id
		LEFT JOIN users rv ON rv.id = lr.reviewed_by
		WHERE lr.id = $1
	`, id).StructScan(&req)

	if err == sql.ErrNoRows {
		middleware.Error(w, http.StatusNotFound, "leave request not found")
		return
	} else if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "database error")
		return
	}

	middleware.JSON(w, http.StatusOK, req)
}

// PUT /api/leave/:id/review — supervisor/hr approves or rejects
func (h *Handler) Review(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	reviewerID := middleware.GetUserID(r)

	var body struct {
		Status     string `json:"status"`      // "approved" or "rejected"
		ReviewNote string `json:"review_note"` // optional reason
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.Status != "approved" && body.Status != "rejected" {
		middleware.Error(w, http.StatusBadRequest, "status must be 'approved' or 'rejected'")
		return
	}

	// Update leave request status
	result, err := h.DB.Exec(`
		UPDATE leave_requests
		SET status = $1, reviewed_by = $2, review_note = $3, updated_at = NOW()
		WHERE id = $4
	`, body.Status, reviewerID, body.ReviewNote, id)

	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to review leave request")
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		middleware.Error(w, http.StatusNotFound, "leave request not found")
		return
	}

	// If approved, update the intern's status to "on_leave"
	if body.Status == "approved" {
		// Get the intern_id from the leave request
		var internID int
		h.DB.QueryRow(`SELECT intern_id FROM leave_requests WHERE id = $1`, id).Scan(&internID)

		// Update intern status in interns table
		h.DB.Exec(`UPDATE interns SET status = 'on_leave' WHERE user_id = $1`, internID)
	}

	middleware.JSON(w, http.StatusOK, map[string]string{
		"message": "leave request " + body.Status,
	})
}

// DELETE /api/leave/:id — intern cancels own request (only if pending)
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	result, err := h.DB.Exec(`
		DELETE FROM leave_requests WHERE id = $1 AND status = 'pending'
	`, id)

	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to cancel leave request")
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		middleware.Error(w, http.StatusNotFound, "leave request not found or already reviewed")
		return
	}

	middleware.JSON(w, http.StatusOK, map[string]string{"message": "leave request cancelled"})
}
