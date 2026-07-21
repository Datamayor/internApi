package announcements

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

type Announcement struct {
	ID         int       `db:"id" json:"id"`
	Title      string    `db:"title" json:"title"`
	Content    string    `db:"content" json:"content"`
	AuthorID   int       `db:"author_id" json:"author_id"`
	Target     string    `db:"target" json:"target"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
	AuthorName string    `db:"author_name" json:"author_name"`
}

// GET /api/announcements
func (h *Handler) GetAll(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("target")

	var announcements []Announcement
	var err error

	if target != "" {
		err = h.DB.Select(&announcements, `
			SELECT a.*, u.name AS author_name
			FROM announcements a
			JOIN users u ON u.id = a.author_id
			WHERE a.target = $1 OR a.target = 'all'
			ORDER BY a.created_at DESC
		`, target)
	} else {
		err = h.DB.Select(&announcements, `
			SELECT a.*, u.name AS author_name
			FROM announcements a
			JOIN users u ON u.id = a.author_id
			ORDER BY a.created_at DESC
		`)
	}

	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to fetch announcements")
		return
	}

	middleware.JSON(w, http.StatusOK, announcements)
}

// GET /api/announcements/:id
func (h *Handler) GetOne(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var a Announcement
	err := h.DB.QueryRowx(`
		SELECT a.*, u.name AS author_name
		FROM announcements a
		JOIN users u ON u.id = a.author_id
		WHERE a.id = $1
	`, id).StructScan(&a)

	if err == sql.ErrNoRows {
		middleware.Error(w, http.StatusNotFound, "announcement not found")
		return
	} else if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "database error")
		return
	}

	middleware.JSON(w, http.StatusOK, a)
}

// POST /api/announcements
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title   string `json:"title"`
		Content string `json:"content"`
		Target  string `json:"target"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.Title == "" || body.Content == "" {
		middleware.Error(w, http.StatusBadRequest, "title and content are required")
		return
	}

	if body.Target == "" {
		body.Target = "all"
	}

	authorID := middleware.GetUserID(r)

	var a Announcement
	err := h.DB.QueryRowx(`
		INSERT INTO announcements (title, content, author_id, target)
		VALUES ($1, $2, $3, $4)
		RETURNING *
	`, body.Title, body.Content, authorID, body.Target).StructScan(&a)

	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to create announcement")
		return
	}

	middleware.JSON(w, http.StatusCreated, a)
}

// DELETE /api/announcements/:id
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	result, err := h.DB.Exec(`DELETE FROM announcements WHERE id = $1`, id)
	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to delete announcement")
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		middleware.Error(w, http.StatusNotFound, "announcement not found")
		return
	}

	middleware.JSON(w, http.StatusOK, map[string]string{"message": "announcement deleted"})
}
