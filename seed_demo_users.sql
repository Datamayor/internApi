-- Seeds the three demo accounts used by the login page's "Demo credentials"
-- shortcuts. Passwords below are bcrypt hashes of the plaintext passwords
-- shown in the login page demo buttons — the plaintext is never stored.
--
-- Run with:
--   go run ./cmd/seed

INSERT INTO users (name, email, password, role)
VALUES
  ('Test User', 'testuser@example.com', '$2b$10$umownMPyaPM7Om7.a2C0Gu4W3/0NJ86p2B7EBUeIuK86YsUhsNsFO', 'intern'),
  ('Demo Supervisor', 'supervisor@example.com', '$2b$10$yHbDvswVJLBnPWoU5XZpuuKChzk9QnhQYUUWw1VE0.1WxcfIOEjzy', 'supervisor'),
  ('Demo Admin', 'admin@example.com', '$2b$10$5VQW9DChUdPZ1opx9mgm1.9VYoYDw3b7LVMY/8q18N1h8nlWxfFIu', 'hr')
ON CONFLICT (email) DO UPDATE
  SET password = EXCLUDED.password,
      role = EXCLUDED.role;
