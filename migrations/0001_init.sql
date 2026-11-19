-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE trips (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  city text NOT NULL,
  days int NOT NULL CHECK (days BETWEEN 1 AND 4),
  budget_yuan int CHECK (budget_yuan >= 0),
  constraints jsonb NOT NULL DEFAULT '{}',
  status text NOT NULL CHECK (status IN ('parsing','planning','awaiting_user','completed','failed','cancelled')),
  request_id text UNIQUE,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
CREATE INDEX trips_created_idx ON trips (created_at DESC);

CREATE TABLE runs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  trip_id uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
  mode text NOT NULL CHECK (mode IN ('initial','adjustment','eval')),
  status text NOT NULL CHECK (status IN ('queued','running','awaiting_user','completed','failed','cancelled')),
  stage text NOT NULL DEFAULT 'admit',
  round int NOT NULL DEFAULT 0,
  input text NOT NULL,
  constraints jsonb NOT NULL DEFAULT '{}',
  base_version_id uuid,
  state jsonb NOT NULL DEFAULT '{}',
  proposal jsonb,
  error_code text,
  error_message text,
  request_id text UNIQUE,
  started_at timestamptz,
  finished_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX runs_trip_idx ON runs (trip_id, created_at DESC);

CREATE TABLE plan_versions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  trip_id uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
  run_id uuid NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  seq int NOT NULL,
  plan jsonb NOT NULL,
  constraints jsonb NOT NULL,
  violations jsonb NOT NULL DEFAULT '[]',
  diff_summary jsonb,
  reason text,
  status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','accepted','superseded')),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (trip_id, seq)
);
CREATE INDEX plan_versions_run_idx ON plan_versions (run_id);

CREATE TABLE run_events (
  id bigserial PRIMARY KEY,
  run_id uuid NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  seq int NOT NULL,
  type text NOT NULL,
  payload jsonb NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (run_id, seq)
);
CREATE INDEX run_events_run_seq_idx ON run_events (run_id, seq);

CREATE TABLE tool_calls (
  id bigserial PRIMARY KEY,
  run_id uuid NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  trip_id uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
  tool text NOT NULL,
  provider text NOT NULL DEFAULT 'native' CHECK (provider IN ('native','mcp')),
  input jsonb NOT NULL,
  output jsonb,
  status text NOT NULL DEFAULT 'ok' CHECK (status IN ('ok','error','timeout','degraded')),
  latency_ms int,
  error text,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX tool_calls_run_idx ON tool_calls (run_id);

CREATE TABLE evidence (
  id bigserial PRIMARY KEY,
  trip_id uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
  run_id uuid REFERENCES runs(id) ON DELETE SET NULL,
  poi_id text,
  kind text NOT NULL,
  claim text NOT NULL,
  source text NOT NULL,
  source_url text,
  confidence numeric(3,2) CHECK (confidence BETWEEN 0 AND 1),
  fetched_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX evidence_trip_idx ON evidence (trip_id);

CREATE TABLE poi_cache (
  id text PRIMARY KEY,
  city text NOT NULL,
  name text NOT NULL,
  category text NOT NULL,
  area text,
  address text,
  lat double precision NOT NULL,
  lng double precision NOT NULL,
  coord_system text NOT NULL DEFAULT 'GCJ-02',
  open_hours jsonb,
  ticket_yuan int,
  suggested_minutes int,
  rating numeric(2,1) CHECK (rating BETWEEN 0 AND 5),
  outdoor boolean NOT NULL DEFAULT false,
  tags text[] NOT NULL DEFAULT '{}',
  source text NOT NULL,
  source_version text,
  verified_at date,
  fetched_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX poi_cache_city_category_idx ON poi_cache (city, category);

CREATE TABLE plugin_cache (
  key text PRIMARY KEY,
  provider text NOT NULL,
  value jsonb NOT NULL,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX plugin_cache_expires_idx ON plugin_cache (expires_at);

CREATE TABLE share_links (
  slug text PRIMARY KEY,
  trip_id uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
  version_id uuid NOT NULL REFERENCES plan_versions(id) ON DELETE CASCADE,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE eval_cases (
  id text PRIMARY KEY,
  input text NOT NULL,
  expected jsonb NOT NULL,
  tags text[] NOT NULL DEFAULT '{}',
  enabled boolean NOT NULL DEFAULT true
);

CREATE TABLE eval_runs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  case_id text NOT NULL,
  run_id uuid REFERENCES runs(id) ON DELETE SET NULL,
  metrics jsonb NOT NULL,
  passed boolean NOT NULL,
  duration_ms int,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX eval_runs_case_idx ON eval_runs (case_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS eval_runs;
DROP TABLE IF EXISTS eval_cases;
DROP TABLE IF EXISTS share_links;
DROP TABLE IF EXISTS plugin_cache;
DROP TABLE IF EXISTS poi_cache;
DROP TABLE IF EXISTS evidence;
DROP TABLE IF EXISTS tool_calls;
DROP TABLE IF EXISTS run_events;
DROP TABLE IF EXISTS plan_versions;
DROP TABLE IF EXISTS runs;
DROP TABLE IF EXISTS trips;
