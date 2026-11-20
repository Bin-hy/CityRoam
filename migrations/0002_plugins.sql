-- +goose Up
ALTER TABLE plan_versions ADD COLUMN research jsonb NOT NULL DEFAULT '{}';

ALTER TABLE runs DROP CONSTRAINT IF EXISTS runs_mode_check;
ALTER TABLE runs ADD CONSTRAINT runs_mode_check
  CHECK (mode IN ('initial', 'adjustment', 'replan', 'eval'));

ALTER TABLE tool_calls DROP CONSTRAINT IF EXISTS tool_calls_provider_check;
ALTER TABLE tool_calls ADD CONSTRAINT tool_calls_provider_check
  CHECK (provider IN ('native', 'mcp', 'plugin'));

-- +goose Down
ALTER TABLE tool_calls DROP CONSTRAINT IF EXISTS tool_calls_provider_check;
ALTER TABLE tool_calls ADD CONSTRAINT tool_calls_provider_check
  CHECK (provider IN ('native', 'mcp'));

ALTER TABLE runs DROP CONSTRAINT IF EXISTS runs_mode_check;
ALTER TABLE runs ADD CONSTRAINT runs_mode_check
  CHECK (mode IN ('initial', 'adjustment', 'eval'));

ALTER TABLE plan_versions DROP COLUMN IF EXISTS research;
