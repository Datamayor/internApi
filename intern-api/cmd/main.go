package main

import (
	"fmt"
	"log"
	"net/http"

	"intern-api/config"
	"intern-api/internal/attendance"
	"intern-api/internal/auth"
	"intern-api/internal/db"
	"intern-api/internal/departments"
	"intern-api/internal/evaluations"
	"intern-api/internal/interns"
	"intern-api/internal/middleware"
	"intern-api/internal/supervisors"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
)

func main() {
	// Load config from .env
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Config error: %v", err)
	}

	// Connect to PostgreSQL
	database := db.Connect(cfg)
	defer database.Close()

	// Create handlers (each handler only needs the DB)
	authHandler := &auth.Handler{
		DB:                    database,
		JWTSecret:             cfg.JWTSecret,
		JWTExpiryHours:        cfg.JWTExpiryHours,
		JWTRefreshExpiryHours: cfg.JWTRefreshExpiryHours,
	}
	internHandler        := &interns.Handler{DB: database}
	deptHandler          := &departments.Handler{DB: database}
	supervisorHandler    := &supervisors.Handler{DB: database}
	attendanceHandler    := &attendance.Handler{DB: database}
	evaluationHandler    := &evaluations.Handler{DB: database}

	// Create router
	r := chi.NewRouter()

	// Global middleware
	r.Use(chiMiddleware.Logger)    // logs every request
	r.Use(chiMiddleware.Recoverer) // recovers from panics
	r.Use(corsMiddleware)          // allow frontend to connect

	// ── Auth routes (no JWT required) ──────────────────────────────────────
	r.Route("/api/auth", func(r chi.Router) {
		r.Post("/register", authHandler.Register)
		r.Post("/login", authHandler.Login)
		r.Post("/refresh-token", authHandler.RefreshToken)

		// These routes need a valid token
		r.Group(func(r chi.Router) {
			r.Use(middleware.Authenticate(cfg.JWTSecret))
			r.Post("/logout", authHandler.Logout)
			r.Get("/profile", authHandler.Profile)
		})
	})

	// ── Protected routes (JWT required) ────────────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(middleware.Authenticate(cfg.JWTSecret))

		// Interns
		r.Get("/api/interns", internHandler.GetAll)
		r.Get("/api/interns/{id}", internHandler.GetOne)
		r.Post("/api/interns", internHandler.Create)
		r.Put("/api/interns/{id}", internHandler.Update)
		r.Delete("/api/interns/{id}", internHandler.Delete)

		// Departments
		r.Get("/api/departments", deptHandler.GetAll)
		r.Get("/api/departments/{id}", deptHandler.GetOne)
		r.Post("/api/departments", deptHandler.Create)
		r.Put("/api/departments/{id}", deptHandler.Update)
		r.Delete("/api/departments/{id}", deptHandler.Delete)

		// Supervisors
		r.Get("/api/supervisors", supervisorHandler.GetAll)
		r.Get("/api/supervisors/{id}", supervisorHandler.GetOne)
		r.Post("/api/supervisors", supervisorHandler.Create)
		r.Put("/api/supervisors/{id}", supervisorHandler.Update)
		r.Delete("/api/supervisors/{id}", supervisorHandler.Delete)

		// Attendance
		r.Get("/api/attendance", attendanceHandler.GetAll)
		r.Post("/api/attendance/check-in", attendanceHandler.CheckIn)
		r.Post("/api/attendance/check-out", attendanceHandler.CheckOut)
		r.Get("/api/attendance/{internId}", attendanceHandler.GetByIntern)

		// Evaluations
		r.Post("/api/evaluations", evaluationHandler.Create)
		r.Get("/api/evaluations", evaluationHandler.GetAll)
		r.Get("/api/evaluations/{internId}", evaluationHandler.GetByIntern)
		r.Put("/api/evaluations/{id}", evaluationHandler.Update)
	})

	// Start server
	addr := fmt.Sprintf(":%s", cfg.ServerPort)
	log.Printf("Server running on http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, r))
}

// corsMiddleware allows requests from any origin (adjust in production)
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// Handle preflight requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
