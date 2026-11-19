-- name: InsertToolCall :one
INSERT INTO tool_calls (run_id, trip_id, tool, provider, input, output, status, latency_ms, error)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: ListToolCallsByRun :many
SELECT * FROM tool_calls
WHERE run_id = $1
ORDER BY created_at, id;
