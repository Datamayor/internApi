package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const UserIDKey contextKey = "userID"
const UserRoleKey contextKey = "userRole"

// Authenticate checks for a valid JWT in the Authorization header
func Authenticate(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
				return
			}

			// Expect: "Bearer <token>"
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				http.Error(w, `{"error":"invalid authorization format"}`, http.StatusUnauthorized)
				return
			}

			tokenString := parts[1]

			token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, jwt.ErrSignatureInvalid
				}
				return []byte(jwtSecret), nil
			})

			if err != nil || !token.Valid {
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				http.Error(w, `{"error":"invalid token claims"}`, http.StatusUnauthorized)
				return
			}

			// Reject anything that isn't an access token — in particular,
			// a refresh token must not be usable as a bearer credential
			// for regular API calls. This is also what makes logout /
			// password-change revocation actually effective, since those
			// only invalidate rows in the refresh_tokens table.
			tokenType, ok := claims["type"].(string)
			if !ok || tokenType != "access" {
				http.Error(w, `{"error":"invalid token type"}`, http.StatusUnauthorized)
				return
			}

			userIDf, ok := claims["user_id"].(float64)
			if !ok {
				http.Error(w, `{"error":"invalid token claims"}`, http.StatusUnauthorized)
				return
			}
			userID := int(userIDf)

			userRole, ok := claims["role"].(string)
			if !ok {
				http.Error(w, `{"error":"invalid token claims"}`, http.StatusUnauthorized)
				return
			}

			// Store user info in context so handlers can read it
			ctx := context.WithValue(r.Context(), UserIDKey, userID)
			ctx = context.WithValue(ctx, UserRoleKey, userRole)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole middleware — only allow certain roles past this point
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role, ok := r.Context().Value(UserRoleKey).(string)
			if !ok {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusForbidden)
				return
			}

			for _, allowed := range roles {
				if role == allowed {
					next.ServeHTTP(w, r)
					return
				}
			}

			http.Error(w, `{"error":"forbidden: insufficient role"}`, http.StatusForbidden)
		})
	}
}

// GetUserID pulls the logged-in user's ID from context
func GetUserID(r *http.Request) int {
	id, _ := r.Context().Value(UserIDKey).(int)
	return id
}
