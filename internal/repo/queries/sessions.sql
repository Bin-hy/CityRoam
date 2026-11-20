-- name: CreateSession :exec
INSERT INTO sessions (id, user_id, expires_at, user_agent, ip)
VALUES ($1, $2, $3, $4, $5);

-- name: GetSessionUser :one
SELECT s.id AS session_id, u.id AS user_id, s.expires_at,
       s.created_at AS session_created_at,
       u.issuer, u.subject, u.email, u.email_verified,
       u.name, u.avatar_url, u.role, u.created_at AS user_created_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.id = $1 AND s.expires_at > now();

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = $1;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions WHERE expires_at <= now();