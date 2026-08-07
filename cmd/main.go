package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"intern-api/config"
	"intern-api/internal/announcements"
	"intern-api/internal/attendance"
	"intern-api/internal/auth"
	"intern-api/internal/db"
	"intern-api/internal/departments"
	"intern-api/internal/evaluations"
	"intern-api/internal/interns"
	"intern-api/internal/internships"
	"intern-api/internal/leave"
	"intern-api/internal/middleware"
	"intern-api/internal/supervisors"
	"intern-api/internal/tasks"
	"intern-api/internal/users"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Config error: %v", err)
	}

	database := db.Connect(cfg)
	defer database.Close()

	authHandler := &auth.Handler{
		DB:                    database,
		JWTSecret:             cfg.JWTSecret,
		JWTExpiryHours:        cfg.JWTExpiryHours,
		JWTRefreshExpiryHours: cfg.JWTRefreshExpiryHours,
		ResendAPIKey:          os.Getenv("RESEND_API_KEY"),
		EmailFrom:             os.Getenv("EMAIL_FROM"),
		FrontendURL:           os.Getenv("FRONTEND_URL"),
	}
	internHandler       := &interns.Handler{DB: database}
	deptHandler         := &departments.Handler{DB: database}
	supervisorHandler   := &supervisors.Handler{DB: database}
	attendanceHandler   := &attendance.Handler{DB: database}
	evaluationHandler   := &evaluations.Handler{DB: database}
	announcementHandler := &announcements.Handler{DB: database}
	internshipHandler   := &internships.Handler{DB: database}
	taskHandler         := &tasks.Handler{DB: database}
	userHandler         := &users.Handler{DB: database}
	leaveHandler        := &leave.Handler{DB: database}

	r := chi.NewRouter()
	r.Use(chiMiddleware.Logger)
	r.Use(chiMiddleware.Recoverer)
	r.Use(corsMiddleware)

	// ── Public routes ───────────────────────────────────────────────────────
	r.Post("/api/auth/register", authHandler.Register)
	r.Post("/api/auth/login", authHandler.Login)
	r.Post("/api/auth/forgot-password", authHandler.ForgotPassword)
	r.Post("/api/auth/reset-password", authHandler.ResetPassword)
	r.Post("/api/auth/refresh-token", authHandler.RefreshToken)
	r.Get("/api/internships", internshipHandler.GetAll)
	r.Get("/api/internships/{id}", internshipHandler.GetOne)

	// ── All authenticated users ─────────────────────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(middleware.Authenticate(cfg.JWTSecret))

		// Auth
		r.Post("/api/auth/logout", authHandler.Logout)
		r.Get("/api/auth/profile", authHandler.Profile)
		r.Put("/api/auth/profile", authHandler.UpdateProfile)
		r.Put("/api/auth/change-password", authHandler.ChangePassword)

		// Read-only for all roles
		r.Get("/api/announcements", announcementHandler.GetAll)
		r.Get("/api/announcements/{id}", announcementHandler.GetOne)
		r.Get("/api/attendance", attendanceHandler.GetAll)
		r.Get("/api/attendance/{internId}", attendanceHandler.GetByIntern)
		r.Get("/api/evaluations", evaluationHandler.GetAll)
		r.Get("/api/evaluations/{internId}", evaluationHandler.GetByIntern)
		r.Get("/api/departments", deptHandler.GetAll)
		r.Get("/api/departments/{id}", deptHandler.GetOne)
		r.Get("/api/supervisors", supervisorHandler.GetAll)
		r.Get("/api/supervisors/{id}", supervisorHandler.GetOne)
		r.Get("/api/interns", internHandler.GetAll)
		r.Get("/api/interns/{id}", internHandler.GetOne)

		// Tasks - all roles can view and update status
		r.Get("/api/tasks", taskHandler.GetAll)
		r.Get("/api/tasks/{id}", taskHandler.GetOne)
		r.Get("/api/tasks/intern/{internId}", taskHandler.GetByIntern)
		r.Put("/api/tasks/{id}/status", taskHandler.UpdateStatus)

		// Leave - all roles can view
		r.Get("/api/leave", leaveHandler.GetAll)
		r.Get("/api/leave/{id}", leaveHandler.GetOne)
		r.Get("/api/leave/intern/{internId}", leaveHandler.GetByIntern)
	})

	// ── Intern + supervisor + hr ────────────────────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(middleware.Authenticate(cfg.JWTSecret))
		r.Use(middleware.RequireRole("intern", "supervisor", "hr"))

		r.Post("/api/attendance/check-in", attendanceHandler.CheckIn)
		r.Post("/api/attendance/check-out", attendanceHandler.CheckOut)

		// Any logged in user can submit and cancel leave requests
		r.Post("/api/leave", leaveHandler.Create)
		r.Delete("/api/leave/{id}", leaveHandler.Delete)
	})

	// ── Supervisor + hr ─────────────────────────────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(middleware.Authenticate(cfg.JWTSecret))
		r.Use(middleware.RequireRole("supervisor", "hr"))

		r.Post("/api/evaluations", evaluationHandler.Create)
		r.Put("/api/evaluations/{id}", evaluationHandler.Update)
		r.Post("/api/interns", internHandler.Create)
		r.Put("/api/interns/{id}", internHandler.Update)
		r.Post("/api/tasks", taskHandler.Create)
		r.Put("/api/tasks/{id}", taskHandler.Update)

		// Approve or reject leave + view intern status overview
		r.Put("/api/leave/{id}/review", leaveHandler.Review)
		r.Get("/api/interns/status", internHandler.GetByStatus)
	})

	// ── HR only ─────────────────────────────────────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(middleware.Authenticate(cfg.JWTSecret))
		r.Use(middleware.RequireRole("hr"))

		r.Post("/api/announcements", announcementHandler.Create)
		r.Delete("/api/announcements/{id}", announcementHandler.Delete)
		r.Delete("/api/interns/{id}", internHandler.Delete)
		r.Post("/api/departments", deptHandler.Create)
		r.Put("/api/departments/{id}", deptHandler.Update)
		r.Delete("/api/departments/{id}", deptHandler.Delete)
		r.Post("/api/supervisors", supervisorHandler.Create)
		r.Put("/api/supervisors/{id}", supervisorHandler.Update)
		r.Delete("/api/supervisors/{id}", supervisorHandler.Delete)
		r.Post("/api/internships", internshipHandler.Create)
		r.Delete("/api/tasks/{id}", taskHandler.Delete)

		// User management — HR only
		r.Get("/api/users", userHandler.GetAll)
		r.Put("/api/users/{id}/role", userHandler.UpdateRole)
	})

	addr := fmt.Sprintf(":%s", cfg.ServerPort)
	log.Printf("Server running on http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, r))
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
