package main

import (
	"context"
	"database/sql"
	"fmt"
	"intern-api/config"
	"intern-api/internal/db"
	"log"
	"os"
	"path/filepath"
	"time"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Config error: %v", err)
	}

	database := db.Connect(cfg)
	defer database.Close()

	root, err := projectRoot()
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	files := []string{
		"schema.sql",
		"migration_password_resets.sql",
		"seed_demo_users.sql",
	}

	for _, name := range files {
		if err := executeSQLFile(ctx, database, filepath.Join(root, name)); err != nil {
			log.Fatalf("Failed to apply %s: %v", name, err)
		}
		log.Printf("Applied %s", name)
	}

	log.Println("Demo users are ready:")
	log.Println("  Intern:     testuser@example.com / password123")
	log.Println("  Supervisor: supervisor@example.com / password12345")
	log.Println("  HR Admin:   admin@example.com / password1234")
}

type sqlExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func executeSQLFile(ctx context.Context, database sqlExecutor, path string) error {
	sqlBytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = database.ExecContext(ctx, string(sqlBytes))
	return err
}

func projectRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(cwd, "schema.sql")); err == nil {
			return cwd, nil
		}

		parent := filepath.Dir(cwd)
		if parent == cwd {
			return "", fmt.Errorf("could not find schema.sql; run this from the internApi folder")
		}
		cwd = parent
	}
}
