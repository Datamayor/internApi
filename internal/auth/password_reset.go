package auth

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"intern-api/internal/middleware"
	"log"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	resetTokenBytes  = 32 // 32 random bytes -> 64 hex chars
	resetTokenExpiry = 1 * time.Hour
)

// genericForgotPasswordMessage is returned whether or not the email exists,
// so the endpoint can't be used to enumerate registered accounts.
const genericForgotPasswordMessage = "if an account with that email exists, a reset link has been sent"

// POST /api/auth/forgot-password
func (h *Handler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	body.Email = strings.ToLower(strings.TrimSpace(body.Email))
	if body.Email == "" {
		middleware.Error(w, http.StatusBadRequest, "email is required")
		return
	}

	ctx := r.Context()

	var user User
	err := h.DB.QueryRowxContext(ctx,
		`SELECT id, name, email, role, created_at FROM users WHERE email = $1`,
		body.Email,
	).StructScan(&user)

	if err != nil {
		// Don't reveal whether the email exists. Log server-side for
		// your own visibility, but respond identically either way.
		middleware.JSON(w, http.StatusOK, map[string]string{"message": genericForgotPasswordMessage})
		return
	}

	token, err := generateResetToken()
	if err != nil {
		log.Printf("auth: failed to generate reset token for user %d: %v", user.ID, err)
		middleware.Error(w, http.StatusInternalServerError, "failed to process request")
		return
	}

	// Invalidate any previous outstanding reset tokens for this user
	// before issuing a new one, so only the latest link is usable.
	if _, err := h.DB.ExecContext(ctx,
		`UPDATE password_resets SET used = TRUE WHERE user_id = $1 AND used = FALSE`,
		user.ID,
	); err != nil {
		log.Printf("auth: failed to invalidate old reset tokens for user %d: %v", user.ID, err)
	}

	if _, err := h.DB.ExecContext(ctx,
		`INSERT INTO password_resets (user_id, token, expires_at) VALUES ($1, $2, $3)`,
		user.ID, token, time.Now().Add(resetTokenExpiry),
	); err != nil {
		log.Printf("auth: failed to store reset token for user %d: %v", user.ID, err)
		middleware.Error(w, http.StatusInternalServerError, "failed to process request")
		return
	}

	resetLink := fmt.Sprintf("%s/reset-password?token=%s", h.FrontendURL, token)

	if err := h.sendResetEmail(user.Email, user.Name, resetLink); err != nil {
		// The token is already saved, so log the failure but still
		// return the generic success message to the client.
		log.Printf("auth: failed to send reset email to user %d: %v", user.ID, err)
	}

	middleware.JSON(w, http.StatusOK, map[string]string{"message": genericForgotPasswordMessage})
}

// POST /api/auth/reset-password
func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	body.Token = strings.TrimSpace(body.Token)
	if body.Token == "" || body.NewPassword == "" {
		middleware.Error(w, http.StatusBadRequest, "token and new_password are required")
		return
	}

	if len(body.NewPassword) < minPasswordLen {
		middleware.Error(w, http.StatusBadRequest, fmt.Sprintf("new password must be at least %d characters", minPasswordLen))
		return
	}

	ctx := r.Context()

	var userID int
	err := h.DB.QueryRowContext(ctx,
		`SELECT user_id FROM password_resets
		 WHERE token = $1 AND used = FALSE AND expires_at > NOW()`,
		body.Token,
	).Scan(&userID)

	if err != nil {
		middleware.Error(w, http.StatusBadRequest, "invalid or expired reset token")
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
		middleware.Error(w, http.StatusInternalServerError, "failed to reset password")
		return
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE password_resets SET used = TRUE WHERE token = $1`,
		body.Token,
	); err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to reset password")
		return
	}

	// Same as change-password: revoke all existing sessions so any
	// stolen/leaked refresh token stops working once the password changes.
	if _, err := tx.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE user_id = $1`, userID); err != nil {
		middleware.Error(w, http.StatusInternalServerError, "failed to revoke sessions")
		return
	}

	if err := tx.Commit(); err != nil {
		middleware.Error(w, http.StatusInternalServerError, "database error")
		return
	}

	middleware.JSON(w, http.StatusOK, map[string]string{"message": "password reset successfully"})
}

// generateResetToken returns a random, URL-safe hex token.
func generateResetToken() (string, error) {
	b := make([]byte, resetTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// sendResetEmail sends the password reset link via the Resend API.
func (h *Handler) sendResetEmail(toEmail, toName, resetLink string) error {
	payload := map[string]any{
		"from":    h.EmailFrom,
		"to":      []string{toEmail},
		"subject": "Reset your password",
		"html": fmt.Sprintf(`
			<p>Hi %s,</p>
			<p>We received a request to reset your password. Click the link below to choose a new one:</p>
			<p><a href="%s">%s</a></p>
			<p>This link expires in 1 hour. If you didn't request this, you can safely ignore this email.</p>
		`, toName, resetLink, resetLink),
	}

	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, "https://api.resend.com/emails", bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+h.ResendAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("resend API returned status %d", resp.StatusCode)
	}

	return nil
}
