-- name: AppendEvent :one
INSERT INTO run_events (run_id, seq, type, payload)
VALUES (
  $1,
  (SELECT COALESCE(MAX(seq), 0) + 1 FROM run_events WHERE run_id = $1),
  $2,
  $3
)
RETURNING *;

-- name: ListEventsAfter :many
SELECT * FROM run_events
WHERE run_id = $1 AND seq > $2
ORDER BY seq;

-- name: ListEvents :many
SELECT * FROM run_events
WHERE run_id = $1
ORDER BY seq;

-- name: CountEvents :one
SELECT count(*) FROM run_events WHERE run_id = $1;
