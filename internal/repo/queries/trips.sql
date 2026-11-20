-- name: CreateTrip :one
INSERT INTO trips (city, days, budget_yuan, constraints, status, request_id, owner_id)
VALUES ($1, $2, $3, $4, $5, $6, $7)
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

-- name: ListTripsByOwner :many
SELECT t.*,
       COALESCE(to_jsonb(r.*), 'null'::jsonb)::jsonb AS latest_run,
       COALESCE(to_jsonb(v.*), 'null'::jsonb)::jsonb AS latest_version
FROM trips t
LEFT JOIN LATERAL (
  SELECT id, status, stage FROM runs
  WHERE trip_id = t.id ORDER BY created_at DESC LIMIT 1
) r ON true
LEFT JOIN LATERAL (
  SELECT seq, status, created_at FROM plan_versions
  WHERE trip_id = t.id ORDER BY seq DESC LIMIT 1
) v ON true
WHERE t.owner_id = $1 AND t.deleted_at IS NULL
ORDER BY t.created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListAnonymousTrips :many
SELECT t.*,
       COALESCE(to_jsonb(r.*), 'null'::jsonb)::jsonb AS latest_run,
       COALESCE(to_jsonb(v.*), 'null'::jsonb)::jsonb AS latest_version
FROM trips t
LEFT JOIN LATERAL (
  SELECT id, status, stage FROM runs
  WHERE trip_id = t.id ORDER BY created_at DESC LIMIT 1
) r ON true
LEFT JOIN LATERAL (
  SELECT seq, status, created_at FROM plan_versions
  WHERE trip_id = t.id ORDER BY seq DESC LIMIT 1
) v ON true
WHERE t.owner_id IS NULL AND t.deleted_at IS NULL
ORDER BY t.created_at DESC
LIMIT $1 OFFSET $2;
