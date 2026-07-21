package auth

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"intern-api/internal/middleware"
	"log"
	"net/http"
	"strings"
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

const (
	defaultRole     = "intern"
	minPasswordLen  = 8
	dummyBcryptHash = "$2a$10$C6UzMDM.H6dfI/f/IKcEeO7Z1uUeM7CqSm3VzS4c1zLmYK1RUIkme" // fixed hash for timing-safe dummy compare
)

// POST /api/auth/register
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	body.Email = strings.ToLower(strings.TrimSpace(body.Email))
	body.Name = strings.TrimSpace(body.Name)

	if body.Name == "" || body.Email == "" || body.Password == "" {
		middleware.Error(w, http.StatusBadRequest, "name, email and password are required")
		return
	}

	if len(body.Password) < minPasswordLen {
		middleware.Error(w, http.StatusBadRequest, fmt.Sprintf("password must be at least %d characters", minPasswordLen))
		return
	}

	// Role is never taken from the client. New accounts are always
	// created as the default role; promotion happens through a
	// separate, admin-only endpoint.
	role := defaultRole

	hashed, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	ctx := r.Context()

	var user User
	err = h.DB.QueryRowxContext(ctx,
		`INSERT INTO users (name, email, password, role) VALUES ($1, $2, $3, $4)
		 RETURNING id, name, email, role, created_at`,
		body.Name, body.Email, string(hashed), role,
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

	body.Email = strings.ToLower(strings.TrimSpace(body.Email))
	ctx := r.Context()

	var user User
	err := h.DB.QueryRowxContext(ctx,
		`SELECT id, name, email, password, role, created_at FROM users WHERE email = $1`,
		body.Email,
	).StructScan(&user)

	if err == sql.ErrNoRows {
		// Run a dummy bcrypt compare so the response time is similar
		// whether or not the account exists (avoids leaking which
		// emails are registered via timing).
		_ = bcrypt.CompareHashAndPassword([]byte(dummyBcryptHash), []byte(body.Password))
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

	accessToken, err := h.generateToken(user.ID, user.Role, h.JWTExpiryHours, "access")
	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	refreshToken, err := h.generateToken(user.ID, user.Role, h.JWTRefreshExpiryHours, "refresh")
	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to generate refresh token")
		return
	}

	if _, err := h.DB.ExecContext(ctx,
		`INSERT INTO refresh_tokens (user_id, token, expires_at) VALUES ($1, $2, $3)`,
		user.ID, refreshToken, time.Now().Add(time.Duration(h.JWTRefreshExpiryHours)*time.Hour),
	); err != nil {
		log.Printf("auth: failed to persist refresh token for user %d: %v", user.ID, err)
		middleware.Error(w, http.StatusInternalServerError, "failed to complete login")
		return
	}

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
	_ = json.NewDecoder(r.Body).Decode(&body)

	if body.RefreshToken != "" {
		if _, err := h.DB.ExecContext(r.Context(),
			`DELETE FROM refresh_tokens WHERE token = $1`, body.RefreshToken,
		); err != nil {
			log.Printf("auth: failed to delete refresh token on logout: %v", err)
		}
	}

	middleware.JSON(w, http.StatusOK, map[string]string{"message": "logged out successfully"})
}

// GET /api/auth/profile
func (h *Handler) Profile(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var user User
	err := h.DB.QueryRowxContext(r.Context(),
		`SELECT id, name, email, role, created_at FROM users WHERE id = $1`, userID,
	).StructScan(&user)
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

	ctx := r.Context()

	// Parse and validate the token, explicitly pinning the expected
	// signing method to prevent algorithm-confusion attacks (e.g. an
	// attacker supplying "alg": "none" or switching HMAC/RSA).
	token, err := jwt.Parse(body.RefreshToken, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(h.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		middleware.Error(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		middleware.Error(w, http.StatusUnauthorized, "invalid token claims")
		return
	}

	userIDf, ok := claims["user_id"].(float64)
	if !ok {
		middleware.Error(w, http.StatusUnauthorized, "invalid token claims")
		return
	}
	role, ok := claims["role"].(string)
	if !ok {
		middleware.Error(w, http.StatusUnauthorized, "invalid token claims")
		return
	}
	userID := int(userIDf)

	// Confirm the token hasn't been revoked (logout / password change)
	// and hasn't expired server-side.
	var count int
	if err := h.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM refresh_tokens WHERE token = $1 AND user_id = $2 AND expires_at > NOW()`,
		body.RefreshToken, userID,
	).Scan(&count); err != nil {
		middleware.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if count == 0 {
		middleware.Error(w, http.StatusUnauthorized, "refresh token not found or expired")
		return
	}

	newAccessToken, err := h.generateToken(userID, role, h.JWTExpiryHours, "access")
	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to generate new token")
		return
	}

	// Rotate the refresh token: issue a new one and invalidate the old
	// one, so a leaked-but-unused token can't be replayed indefinitely.
	newRefreshToken, err := h.generateToken(userID, role, h.JWTRefreshExpiryHours, "refresh")
	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to generate refresh token")
		return
	}

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE token = $1`, body.RefreshToken); err != nil {
		middleware.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO refresh_tokens (user_id, token, expires_at) VALUES ($1, $2, $3)`,
		userID, newRefreshToken, time.Now().Add(time.Duration(h.JWTRefreshExpiryHours)*time.Hour),
	); err != nil {
		middleware.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if err := tx.Commit(); err != nil {
		middleware.Error(w, http.StatusInternalServerError, "database error")
		return
	}

	middleware.JSON(w, http.StatusOK, map[string]string{
		"access_token":  newAccessToken,
		"refresh_token": newRefreshToken,
	})
}

// generateToken creates a signed JWT
func (h *Handler) generateToken(userID int, role string, expiryHours int, tokenType string) (string, error) {
	claims := jwt.MapClaims{
		"user_id": userID,
		"role":    role,
		"type":    tokenType,
		"exp":     time.Now().Add(time.Duration(expiryHours) * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(h.JWTSecret))
}

// PUT /api/auth/profile — update name
func (h *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var body struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		middleware.Error(w, http.StatusBadRequest, "name is required")
		return
	}

	_, err := h.DB.ExecContext(r.Context(),
		`UPDATE users SET name = $1, updated_at = NOW() WHERE id = $2`,
		body.Name, userID,
	)
	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to update profile")
		return
	}

	middleware.JSON(w, http.StatusOK, map[string]string{"message": "profile updated"})
}

// PUT /api/auth/change-password
func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	ctx := r.Context()

	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.CurrentPassword == "" || body.NewPassword == "" {
		middleware.Error(w, http.StatusBadRequest, "current_password and new_password are required")
		return
	}

	if len(body.NewPassword) < minPasswordLen {
		middleware.Error(w, http.StatusBadRequest, fmt.Sprintf("new password must be at least %d characters", minPasswordLen))
		return
	}

	var currentHash string
	if err := h.DB.QueryRowContext(ctx, `SELECT password FROM users WHERE id = $1`, userID).Scan(&currentHash); err != nil {
		middleware.Error(w, http.StatusInternalServerError, "database error")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(body.CurrentPassword)); err != nil {
		middleware.Error(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		middleware.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`UPDATE users SET password = $1, updated_at = NOW() WHERE id = $2`,
		string(newHash), userID,
	); err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to change password")
		return
	}

	// Revoke all existing sessions so a previously leaked/stolen
	// refresh token stops working once the password is changed.
	if _, err := tx.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE user_id = $1`, userID); err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to revoke sessions")
		return
	}

	if err := tx.Commit(); err != nil {
		middleware.Error(w, http.StatusInternalServerError, "database error")
		return
	}

	middleware.JSON(w, http.StatusOK, map[string]string{"message": "password changed successfully"})
}
