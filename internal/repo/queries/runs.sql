-- name: CreateRun :one
INSERT INTO runs (trip_id, mode, status, stage, input, constraints, base_version_id, request_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetRun :one
SELECT * FROM runs WHERE id = $1;

-- name: GetRunByRequestID :one
SELECT * FROM runs WHERE request_id = $1;

-- name: ListRunsByStatus :many
SELECT * FROM runs
WHERE status = ANY($1::text[])
ORDER BY created_at;

-- name: ListRunsByTrip :many
SELECT * FROM runs
WHERE trip_id = $1
ORDER BY created_at DESC;

-- name: UpdateRunState :exec
UPDATE runs
SET state = $2,
    stage = $3,
    round = $4,
    status = $5,
    proposal = $6,
    error_code = $7,
    error_message = $8,
    started_at = COALESCE(started_at, $9),
    finished_at = $10
WHERE id = $1;

-- name: UpdateRunStatus :exec
UPDATE runs
SET status = $2,
    stage = $3,
    error_code = $4,
    error_message = $5,
    finished_at = $6
WHERE id = $1;

-- name: MarkRunStarted :exec
UPDATE runs
SET status = 'running', started_at = COALESCE(started_at, now())
WHERE id = $1;
