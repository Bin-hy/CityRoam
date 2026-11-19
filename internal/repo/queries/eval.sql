-- name: ListEvalCases :many
SELECT * FROM eval_cases
WHERE enabled
ORDER BY id;

-- name: ListAllEvalCases :many
SELECT * FROM eval_cases
ORDER BY id;

-- name: GetEvalCase :one
SELECT * FROM eval_cases WHERE id = $1;

-- name: UpsertEvalCase :exec
INSERT INTO eval_cases (id, input, expected, tags, enabled)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (id) DO UPDATE SET
  input = EXCLUDED.input,
  expected = EXCLUDED.expected,
  tags = EXCLUDED.tags,
  enabled = EXCLUDED.enabled;

-- name: InsertEvalRun :one
INSERT INTO eval_runs (case_id, run_id, metrics, passed, duration_ms)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListEvalRunsByCase :many
SELECT * FROM eval_runs
WHERE case_id = $1
ORDER BY created_at DESC;

-- name: ListEvalRuns :many
SELECT * FROM eval_runs
ORDER BY created_at DESC
LIMIT 200;
