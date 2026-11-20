-- name: UpsertPOI :exec
INSERT INTO poi_cache (
  id, city, name, category, area, address, lat, lng, coord_system,
  open_hours, ticket_yuan, suggested_minutes, rating, outdoor, tags,
  source, source_version, verified_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9,
  $10, $11, $12, $13, $14, $15,
  $16, $17, $18
)
ON CONFLICT (id) DO UPDATE SET
  city = EXCLUDED.city,
  name = EXCLUDED.name,
  category = EXCLUDED.category,
  area = EXCLUDED.area,
  address = EXCLUDED.address,
  lat = EXCLUDED.lat,
  lng = EXCLUDED.lng,
  coord_system = EXCLUDED.coord_system,
  open_hours = EXCLUDED.open_hours,
  ticket_yuan = EXCLUDED.ticket_yuan,
  suggested_minutes = EXCLUDED.suggested_minutes,
  rating = EXCLUDED.rating,
  outdoor = EXCLUDED.outdoor,
  tags = EXCLUDED.tags,
  source = EXCLUDED.source,
  source_version = EXCLUDED.source_version,
  verified_at = EXCLUDED.verified_at,
  fetched_at = now();

-- name: GetPOI :one
SELECT * FROM poi_cache WHERE id = $1;

-- name: ListPOIsByCity :many
SELECT * FROM poi_cache
WHERE city = $1
ORDER BY id;

-- name: ListPOIsByCityAndCategory :many
SELECT * FROM poi_cache
WHERE city = $1 AND category = $2
ORDER BY id;

-- name: CountPOIs :one
SELECT count(*) FROM poi_cache WHERE city = $1;
