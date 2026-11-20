-- name: GetPluginCache :one
SELECT * FROM plugin_cache
WHERE key = $1;

-- name: SetPluginCache :exec
INSERT INTO plugin_cache (key, provider, value, expires_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (key) DO UPDATE
SET provider = EXCLUDED.provider,
    value = EXCLUDED.value,
    expires_at = EXCLUDED.expires_at;

-- name: DeleteExpiredPluginCache :exec
DELETE FROM plugin_cache WHERE expires_at <= now();
