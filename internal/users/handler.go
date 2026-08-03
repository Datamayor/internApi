package users

import (
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

type User struct {
	ID        int       `db:"id" json:"id"`
	Name      string    `db:"name" json:"name"`
	Email     string    `db:"email" json:"email"`
	Role      string    `db:"role" json:"role"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// GET /api/users — get all registered users (hr only)
func (h *Handler) GetAll(w http.ResponseWriter, r *http.Request) {
	var allUsers []User
	err := h.DB.Select(&allUsers, `
		SELECT id, name, email, role, created_at
		FROM users
		ORDER BY created_at DESC
	`)
	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to fetch users")
		return
	}
	middleware.JSON(w, http.StatusOK, allUsers)
}

// PUT /api/users/:id/role — promote or demote a user (hr only)
func (h *Handler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var body struct {
		Role string `json:"role"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	validRoles := map[string]bool{
		"intern": true, "supervisor": true, "hr": true,
	}
	if !validRoles[body.Role] {
		middleware.Error(w, http.StatusBadRequest, "role must be: intern, supervisor, or hr")
		return
	}

	// Prevent HR from changing their own role
	requestingUserID := middleware.GetUserID(r)
	var targetID int
	err := h.DB.QueryRow(`SELECT id FROM users WHERE id = $1`, id).Scan(&targetID)
	if err != nil {
		middleware.Error(w, http.StatusNotFound, "user not found")
		return
	}

	if requestingUserID == targetID {
		middleware.Error(w, http.StatusForbidden, "you cannot change your own role")
		return
	}

	result, err := h.DB.Exec(
		`UPDATE users SET role = $1, updated_at = NOW() WHERE id = $2`,
		body.Role, id,
	)
	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to update role")
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		middleware.Error(w, http.StatusNotFound, "user not found")
		return
	}

	middleware.JSON(w, http.StatusOK, map[string]string{
		"message": "user role updated to " + body.Role,
	})
}
