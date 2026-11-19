-- name: CreateShareLink :one
INSERT INTO share_links (slug, trip_id, version_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetShareLink :one
SELECT * FROM share_links
WHERE slug = $1 AND revoked_at IS NULL;

-- name: RevokeShareLink :exec
UPDATE share_links
SET revoked_at = now()
WHERE slug = $1 AND revoked_at IS NULL;
