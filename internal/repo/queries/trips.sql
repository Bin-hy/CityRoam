-- name: CreateTrip :one
INSERT INTO trips (city, days, budget_yuan, constraints, status, request_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetTrip :one
SELECT * FROM trips
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetTripByRequestID :one
SELECT * FROM trips
WHERE request_id = $1 AND deleted_at IS NULL;

-- name: UpdateTripStatus :exec
UPDATE trips
SET status = $2, updated_at = now()
WHERE id = $1;

-- name: SoftDeleteTrip :exec
UPDATE trips
SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;
