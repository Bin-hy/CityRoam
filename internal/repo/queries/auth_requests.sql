-- name: CreateAuthRequest :exec
INSERT INTO auth_requests (state, nonce, code_verifier, return_to, expires_at)
VALUES ($1, $2, $3, $4, $5);

-- name: ConsumeAuthRequest :one
DELETE FROM auth_requests
WHERE state = $1 AND expires_at > now()
RETURNING *;

-- name: DeleteExpiredAuthRequests :execrows
DELETE FROM auth_requests WHERE expires_at <= now();