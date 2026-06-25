package auth

import (
	"database/sql"
	"encoding/json"
	"intern-api/internal/middleware"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"
)

type Handler struct {
	DB                    *sqlx.DB
	JWTSecret             string
	JWTExpiryHours        int
	JWTRefreshExpiryHours int
}

type User struct {
	ID        int       `db:"id" json:"id"`
	Name      string    `db:"name" json:"name"`
	Email     string    `db:"email" json:"email"`
	Password  string    `db:"password" json:"-"` // never send password in response
	Role      string    `db:"role" json:"role"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// POST /api/auth/register
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.Name == "" || body.Email == "" || body.Password == "" {
		middleware.Error(w, http.StatusBadRequest, "name, email and password are required")
		return
	}

	// Default role to intern if not provided
	if body.Role == "" {
		body.Role = "intern"
	}

	// Hash the password
	hashed, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	var user User
	err = h.DB.QueryRowx(
		`INSERT INTO users (name, email, password, role) VALUES ($1, $2, $3, $4)
		 RETURNING id, name, email, role, created_at`,
		body.Name, body.Email, string(hashed), body.Role,
	).StructScan(&user)

	if err != nil {
		// Likely a duplicate email
		middleware.Error(w, http.StatusConflict, "email already in use")
		return
	}

	middleware.JSON(w, http.StatusCreated, user)
}

// POST /api/auth/login
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var user User
	err := h.DB.QueryRowx(`SELECT * FROM users WHERE email = $1`, body.Email).StructScan(&user)
	if err == sql.ErrNoRows {
		middleware.Error(w, http.StatusUnauthorized, "invalid email or password")
		return
	} else if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "database error")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(body.Password)); err != nil {
		middleware.Error(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	accessToken, err := h.generateToken(user.ID, user.Role, h.JWTExpiryHours)
	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	refreshToken, err := h.generateToken(user.ID, user.Role, h.JWTRefreshExpiryHours)
	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to generate refresh token")
		return
	}

	// Save refresh token in DB
	_, _ = h.DB.Exec(
		`INSERT INTO refresh_tokens (user_id, token, expires_at) VALUES ($1, $2, $3)`,
		user.ID, refreshToken, time.Now().Add(time.Duration(h.JWTRefreshExpiryHours)*time.Hour),
	)

	middleware.JSON(w, http.StatusOK, map[string]any{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"user":          user,
	})
}

// POST /api/auth/logout
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	// Delete the refresh token from DB
	if body.RefreshToken != "" {
		h.DB.Exec(`DELETE FROM refresh_tokens WHERE token = $1`, body.RefreshToken)
	}

	middleware.JSON(w, http.StatusOK, map[string]string{"message": "logged out successfully"})
}

// GET /api/auth/profile
func (h *Handler) Profile(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var user User
	err := h.DB.QueryRowx(`SELECT id, name, email, role, created_at FROM users WHERE id = $1`, userID).StructScan(&user)
	if err != nil {
		middleware.Error(w, http.StatusNotFound, "user not found")
		return
	}

	middleware.JSON(w, http.StatusOK, user)
}

// POST /api/auth/refresh-token
func (h *Handler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RefreshToken == "" {
		middleware.Error(w, http.StatusBadRequest, "refresh_token is required")
		return
	}

	// Validate the token
	token, err := jwt.Parse(body.RefreshToken, func(t *jwt.Token) (interface{}, error) {
		return []byte(h.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		middleware.Error(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}

	claims, _ := token.Claims.(jwt.MapClaims)
	userID := int(claims["user_id"].(float64))
	role := claims["role"].(string)

	// Check if this refresh token exists in DB
	var count int
	h.DB.QueryRow(`SELECT COUNT(*) FROM refresh_tokens WHERE token = $1 AND expires_at > NOW()`, body.RefreshToken).Scan(&count)
	if count == 0 {
		middleware.Error(w, http.StatusUnauthorized, "refresh token not found or expired")
		return
	}

	// Issue new access token
	newAccessToken, err := h.generateToken(userID, role, h.JWTExpiryHours)
	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to generate new token")
		return
	}

	middleware.JSON(w, http.StatusOK, map[string]string{"access_token": newAccessToken})
}

// generateToken creates a signed JWT
func (h *Handler) generateToken(userID int, role string, expiryHours int) (string, error) {
	claims := jwt.MapClaims{
		"user_id": userID,
		"role":    role,
		"exp":     time.Now().Add(time.Duration(expiryHours) * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(h.JWTSecret))
}
