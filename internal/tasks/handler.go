package tasks

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

type Task struct {
	ID           int        `db:"id" json:"id"`
	Title        string     `db:"title" json:"title"`
	Description  string     `db:"description" json:"description"`
	InternID     *int       `db:"intern_id" json:"intern_id"`
	AssignedBy   int        `db:"assigned_by" json:"assigned_by"`
	Status       string     `db:"status" json:"status"` // pending, in_progress, completed, blocked
	Priority     string     `db:"priority" json:"priority"` // low, medium, high
	DueDate      *time.Time `db:"due_date" json:"due_date"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time  `db:"updated_at" json:"updated_at"`
	// Joined fields
	InternName   string     `db:"intern_name" json:"intern_name"`
	AssignerName string     `db:"assigner_name" json:"assigner_name"`
}

// GET /api/tasks
func (h *Handler) GetAll(w http.ResponseWriter, r *http.Request) {
	var tasks []Task
	err := h.DB.Select(&tasks, `
		SELECT t.*,
			COALESCE(u.name, '') AS intern_name,
			COALESCE(a.name, '') AS assigner_name
		FROM tasks t
		LEFT JOIN users u ON u.id = t.intern_id
		LEFT JOIN users a ON a.id = t.assigned_by
		ORDER BY t.created_at DESC
	`)
	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to fetch tasks")
		return
	}
	middleware.JSON(w, http.StatusOK, tasks)
}

// GET /api/tasks/:id
func (h *Handler) GetOne(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var task Task
	err := h.DB.QueryRowx(`
		SELECT t.*,
			COALESCE(u.name, '') AS intern_name,
			COALESCE(a.name, '') AS assigner_name
		FROM tasks t
		LEFT JOIN users u ON u.id = t.intern_id
		LEFT JOIN users a ON a.id = t.assigned_by
		WHERE t.id = $1
	`, id).StructScan(&task)

	if err == sql.ErrNoRows {
		middleware.Error(w, http.StatusNotFound, "task not found")
		return
	} else if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "database error")
		return
	}

	middleware.JSON(w, http.StatusOK, task)
}

// GET /api/tasks/intern/:internId
func (h *Handler) GetByIntern(w http.ResponseWriter, r *http.Request) {
	internID := chi.URLParam(r, "internId")

	var tasks []Task
	err := h.DB.Select(&tasks, `
		SELECT t.*,
			COALESCE(u.name, '') AS intern_name,
			COALESCE(a.name, '') AS assigner_name
		FROM tasks t
		LEFT JOIN users u ON u.id = t.intern_id
		LEFT JOIN users a ON a.id = t.assigned_by
		WHERE t.intern_id = $1
		ORDER BY t.created_at DESC
	`, internID)

	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to fetch tasks")
		return
	}
	middleware.JSON(w, http.StatusOK, tasks)
}

// POST /api/tasks
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		InternID    *int   `json:"intern_id"`
		Status      string `json:"status"`
		Priority    string `json:"priority"`
		DueDate     string `json:"due_date"` // YYYY-MM-DD
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.Title == "" {
		middleware.Error(w, http.StatusBadRequest, "title is required")
		return
	}

	if body.Status == "" {
		body.Status = "pending"
	}

	if body.Priority == "" {
		body.Priority = "medium"
	}

	assignedBy := middleware.GetUserID(r)

	var task Task
	err := h.DB.QueryRowx(`
		INSERT INTO tasks (title, description, intern_id, assigned_by, status, priority, due_date)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING *
	`, body.Title, body.Description, body.InternID, assignedBy, body.Status, body.Priority, nullString(body.DueDate)).StructScan(&task)

	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to create task")
		return
	}

	middleware.JSON(w, http.StatusCreated, task)
}

// PUT /api/tasks/:id
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var body struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		InternID    *int   `json:"intern_id"`
		Status      string `json:"status"`
		Priority    string `json:"priority"`
		DueDate     string `json:"due_date"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := h.DB.Exec(`
		UPDATE tasks
		SET title = $1, description = $2, intern_id = $3, status = $4, priority = $5, due_date = $6, updated_at = NOW()
		WHERE id = $7
	`, body.Title, body.Description, body.InternID, body.Status, body.Priority, nullString(body.DueDate), id)

	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to update task")
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		middleware.Error(w, http.StatusNotFound, "task not found")
		return
	}

	middleware.JSON(w, http.StatusOK, map[string]string{"message": "task updated"})
}

// PUT /api/tasks/:id/status — quick status update
func (h *Handler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var body struct {
		Status string `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	validStatuses := map[string]bool{
		"pending": true, "in_progress": true, "completed": true, "blocked": true,
	}
	if !validStatuses[body.Status] {
		middleware.Error(w, http.StatusBadRequest, "status must be: pending, in_progress, completed, or blocked")
		return
	}

	result, err := h.DB.Exec(`
		UPDATE tasks SET status = $1, updated_at = NOW() WHERE id = $2
	`, body.Status, id)

	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to update task status")
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		middleware.Error(w, http.StatusNotFound, "task not found")
		return
	}

	middleware.JSON(w, http.StatusOK, map[string]string{"message": "task status updated"})
}

// DELETE /api/tasks/:id
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	result, err := h.DB.Exec(`DELETE FROM tasks WHERE id = $1`, id)
	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to delete task")
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		middleware.Error(w, http.StatusNotFound, "task not found")
		return
	}

	middleware.JSON(w, http.StatusOK, map[string]string{"message": "task deleted"})
}

func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
