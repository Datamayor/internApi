# D'accubin Interns API

## Local database setup

This repo does not require a local `psql` install. Use Docker for Postgres, then run the Go seeder.

```powershell
cd C:\Users\nsika\intern\internApi
docker compose up -d db
go run ./cmd/seed
```

The Compose database runs on `localhost:5432` and creates `intern_db`. On a brand-new Docker volume it also applies `schema.sql`, `migration_password_resets.sql`, and `seed_demo_users.sql` automatically.

If `docker` is not recognized in PowerShell, Docker Desktop is not installed or its CLI is not on PATH. Install/start Docker Desktop, then open a new PowerShell window and run:

```powershell
docker --version
```

Once seeding completes, the demo logins are:

```text
Intern:     testuser@example.com / password123
Supervisor: supervisor@example.com / password12345
HR Admin:   admin@example.com / password1234
```

Start the API locally with:

```powershell
go run ./cmd/main.go
```

Or run both Postgres and the API in Docker:

```powershell
docker compose up --build
```
