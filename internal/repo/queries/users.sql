-- name: UpsertUser :one
INSERT INTO users (issuer, subject, email, email_verified, name, avatar_url, role)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (issuer, subject) DO UPDATE SET
  email = EXCLUDED.email,
  email_verified = EXCLUDED.email_verified,
  name = EXCLUDED.name,
  avatar_url = EXCLUDED.avatar_url,
  role = EXCLUDED.role,
  updated_at = now(),
  last_login_at = now()
RETURNING *;

-- name: GetUserByIssuerSubject :one
SELECT * FROM users WHERE issuer = $1 AND subject = $2;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;