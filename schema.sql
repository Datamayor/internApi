-- Run this file in your PostgreSQL database to set up all tables
-- psql -U postgres -d intern_db -f schema.sql

CREATE TABLE IF NOT EXISTS users (
    id          SERIAL PRIMARY KEY,
    name        VARCHAR(100) NOT NULL,
    email       VARCHAR(100) UNIQUE NOT NULL,
    password    VARCHAR(255) NOT NULL,
    role        VARCHAR(20) NOT NULL DEFAULT 'intern', -- 'admin', 'supervisor', 'intern'
    created_at  TIMESTAMP DEFAULT NOW(),
    updated_at  TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS departments (
    id          SERIAL PRIMARY KEY,
    name        VARCHAR(100) NOT NULL,
    description TEXT,
    created_at  TIMESTAMP DEFAULT NOW(),
    updated_at  TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS supervisors (
    id            SERIAL PRIMARY KEY,
    user_id       INT REFERENCES users(id) ON DELETE CASCADE,
    department_id INT REFERENCES departments(id) ON DELETE SET NULL,
    phone         VARCHAR(20),
    created_at    TIMESTAMP DEFAULT NOW(),
    updated_at    TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS interns (
    id            SERIAL PRIMARY KEY,
    user_id       INT REFERENCES users(id) ON DELETE CASCADE,
    department_id INT REFERENCES departments(id) ON DELETE SET NULL,
    supervisor_id INT REFERENCES supervisors(id) ON DELETE SET NULL,
    start_date    DATE,
    end_date      DATE,
    status        VARCHAR(20) DEFAULT 'active', -- 'active', 'completed', 'terminated'
    created_at    TIMESTAMP DEFAULT NOW(),
    updated_at    TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS attendance (
    id         SERIAL PRIMARY KEY,
    intern_id  INT REFERENCES interns(id) ON DELETE CASCADE,
    date       DATE NOT NULL,
    check_in   TIMESTAMP,
    check_out  TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS evaluations (
    id            SERIAL PRIMARY KEY,
    intern_id     INT REFERENCES interns(id) ON DELETE CASCADE,
    supervisor_id INT REFERENCES supervisors(id) ON DELETE SET NULL,
    score         INT CHECK (score >= 1 AND score <= 10),
    comments      TEXT,
    period        VARCHAR(50), -- e.g. 'Week 1', 'Month 1'
    created_at    TIMESTAMP DEFAULT NOW(),
    updated_at    TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id         SERIAL PRIMARY KEY,
    user_id    INT REFERENCES users(id) ON DELETE CASCADE,
    token      VARCHAR(500) NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);


CREATE TABLE IF NOT EXISTS announcements (
    id         SERIAL PRIMARY KEY,
    title      VARCHAR(200) NOT NULL,
    content    TEXT NOT NULL,
    author_id  INT REFERENCES users(id) ON DELETE SET NULL,
    target     VARCHAR(20) DEFAULT 'all', -- 'all', 'intern', 'staff'
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS internships (
    id          SERIAL PRIMARY KEY,
    title       VARCHAR(200) NOT NULL,
    description TEXT,
    department  VARCHAR(100),
    duration    VARCHAR(100),
    location    VARCHAR(100),
    is_open     BOOLEAN DEFAULT TRUE,
    created_at  TIMESTAMP DEFAULT NOW(),
    updated_at  TIMESTAMP DEFAULT NOW()
);