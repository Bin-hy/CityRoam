-- name: NextPlanVersionSeq :one
SELECT COALESCE(MAX(seq), 0) + 1 FROM plan_versions WHERE trip_id = $1;

-- name: CreatePlanVersion :one
INSERT INTO plan_versions (trip_id, run_id, seq, plan, constraints, violations, diff_summary, reason, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetPlanVersion :one
SELECT * FROM plan_versions WHERE id = $1;

-- name: GetPlanVersionBySeq :one
SELECT * FROM plan_versions WHERE trip_id = $1 AND seq = $2;

-- name: ListPlanVersions :many
SELECT * FROM plan_versions
WHERE trip_id = $1
ORDER BY seq DESC;

-- name: SetPlanVersionStatus :exec
UPDATE plan_versions SET status = $2 WHERE id = $1;
