-- name: InsertEvidence :one
INSERT INTO evidence (trip_id, run_id, poi_id, kind, claim, source, source_url, confidence)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListEvidenceByTrip :many
SELECT * FROM evidence
WHERE trip_id = $1
ORDER BY fetched_at, id;

-- name: ListEvidenceByRun :many
SELECT * FROM evidence
WHERE run_id = $1
ORDER BY fetched_at, id;
