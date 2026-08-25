-- Reusable SQLite fixture for the API key analytics dashboard.
-- Run only against an isolated test database. Re-running replaces rows marked
-- with the fixed Analytics Seed prefixes and leaves unrelated rows untouched.

PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;

BEGIN IMMEDIATE;

CREATE TEMP TABLE analytics_seed_context (
  project_id INTEGER NOT NULL
);

INSERT INTO analytics_seed_context (project_id)
VALUES (
  (
    SELECT id
    FROM projects
    WHERE deleted_at = 0 AND status = 'active'
    ORDER BY id
    LIMIT 1
  )
);

DELETE FROM usage_logs
WHERE request_id IN (
  SELECT id
  FROM requests
  WHERE external_id LIKE 'api-key-analytics-seed:%'
);

DELETE FROM requests
WHERE external_id LIKE 'api-key-analytics-seed:%';

DELETE FROM api_keys
WHERE key LIKE 'ah_api_key_analytics_seed_%';

DELETE FROM api_key_profile_templates
WHERE name LIKE '[Analytics Seed] %'
  AND project_id = (
    SELECT project_id
    FROM analytics_seed_context
  );

WITH seed_templates(name) AS (
  VALUES ('Premium'), ('Standard')
)
INSERT INTO api_key_profile_templates (
  name,
  description,
  profile,
  project_id
)
SELECT
  '[Analytics Seed] ' || templates.name,
  'Mock template for API key analytics',
  json_object(
    'name', templates.name,
    'modelMappings', json('[]')
  ),
  context.project_id
FROM seed_templates AS templates
CROSS JOIN analytics_seed_context AS context;

WITH seed_keys(key_suffix, name, template_name) AS (
  VALUES
    ('premium', 'High Cost / Few Requests', 'Premium'),
    ('standard', 'Low Cost / Many Requests', 'Standard'),
    ('custom', 'Medium Cost / No Template', NULL)
)
INSERT INTO api_keys (
  key,
  name,
  type,
  status,
  scopes,
  profiles,
  allowed_ips,
  project_id
)
SELECT
  'ah_api_key_analytics_seed_' || keys.key_suffix,
  '[Analytics Seed] ' || keys.name,
  'service_account',
  'enabled',
  json('["read_channels","write_requests"]'),
  CASE
    WHEN keys.template_name IS NULL THEN json_object(
      'activeProfile', 'Custom',
      'profiles', json_array(
        json_object(
          'name', 'Custom',
          'modelMappings', json('[]')
        )
      )
    )
    ELSE json_object(
      'activeProfile', keys.template_name,
      'profiles', json_array(
        json_object(
          'name', keys.template_name,
          'templateID', templates.id,
          'templateName', templates.name,
          'modelMappings', json('[]')
        )
      )
    )
  END,
  json('[]'),
  context.project_id
FROM seed_keys AS keys
CROSS JOIN analytics_seed_context AS context
LEFT JOIN api_key_profile_templates AS templates
  ON templates.project_id = context.project_id
  AND templates.name = '[Analytics Seed] ' || keys.template_name
  AND templates.deleted_at = 0;

CREATE TEMP TABLE analytics_seed_rows (
  suffix TEXT PRIMARY KEY,
  key_suffix TEXT NOT NULL,
  model_id TEXT NOT NULL,
  day_offset INTEGER NOT NULL,
  hour INTEGER NOT NULL,
  prompt_tokens INTEGER NOT NULL,
  completion_tokens INTEGER NOT NULL,
  total_cost REAL NOT NULL
);

INSERT INTO analytics_seed_rows VALUES
  ('p-today-opus', 'premium', 'claude-opus-mock', 0, 12, 2000, 1000, 9.0),
  ('p-today-gpt', 'premium', 'gpt-5-mock', 0, 13, 700, 300, 3.5),
  ('p-yesterday', 'premium', 'claude-opus-mock', -1, 12, 700, 300, 2.5),
  ('s-today-mini-1', 'standard', 'gpt-mini-mock', 0, 10, 600, 200, 0.4),
  ('s-today-mini-2', 'standard', 'gpt-mini-mock', 0, 11, 600, 200, 0.4),
  ('s-today-mini-3', 'standard', 'gpt-mini-mock', 0, 12, 600, 200, 0.4),
  ('s-today-mini-4', 'standard', 'gpt-mini-mock', 0, 13, 600, 200, 0.4),
  ('s-today-emb-1', 'standard', 'embedding-mock', 0, 14, 400, 0, 0.2),
  ('s-today-emb-2', 'standard', 'embedding-mock', 0, 15, 400, 0, 0.2),
  ('s-yesterday-1', 'standard', 'gpt-mini-mock', -1, 11, 500, 100, 0.5),
  ('s-yesterday-2', 'standard', 'gpt-mini-mock', -1, 12, 500, 100, 0.5),
  ('c-today-1', 'custom', 'batch-model-mock', 0, 9, 800, 200, 2.0),
  ('c-today-2', 'custom', 'batch-model-mock', 0, 10, 800, 200, 2.0),
  ('c-today-3', 'custom', 'batch-model-mock', 0, 11, 800, 200, 2.0),
  ('c-last-month', 'custom', 'batch-model-mock', -99, 12, 1600, 400, 2.0);

CREATE TEMP VIEW analytics_seed_data AS
SELECT
  rows.*,
  CASE rows.day_offset
    WHEN -99 THEN datetime(
      date('now', 'start of month', '-1 day'),
      printf('+%d hours', rows.hour)
    )
    ELSE datetime(
      date('now', printf('%+d days', rows.day_offset)),
      printf('+%d hours', rows.hour)
    )
  END AS created_at
FROM analytics_seed_rows AS rows;

INSERT INTO requests (
  created_at,
  updated_at,
  source,
  model_id,
  request_body,
  external_id,
  status,
  api_key_id,
  project_id
)
SELECT
  rows.created_at,
  rows.created_at,
  'test',
  rows.model_id,
  json('{"seed":true}'),
  'api-key-analytics-seed:' || rows.suffix,
  'completed',
  keys.id,
  context.project_id
FROM analytics_seed_data AS rows
JOIN api_keys AS keys
  ON keys.key = 'ah_api_key_analytics_seed_' || rows.key_suffix
CROSS JOIN analytics_seed_context AS context;

INSERT INTO usage_logs (
  created_at,
  updated_at,
  api_key_id,
  model_id,
  prompt_tokens,
  completion_tokens,
  total_tokens,
  source,
  total_cost,
  cost_items,
  project_id,
  request_id
)
SELECT
  rows.created_at,
  rows.created_at,
  keys.id,
  rows.model_id,
  rows.prompt_tokens,
  rows.completion_tokens,
  rows.prompt_tokens + rows.completion_tokens,
  'test',
  rows.total_cost,
  json('[]'),
  context.project_id,
  requests.id
FROM analytics_seed_data AS rows
JOIN api_keys AS keys
  ON keys.key = 'ah_api_key_analytics_seed_' || rows.key_suffix
JOIN requests
  ON requests.external_id = 'api-key-analytics-seed:' || rows.suffix
CROSS JOIN analytics_seed_context AS context;

DROP VIEW analytics_seed_data;
DROP TABLE analytics_seed_rows;
DROP TABLE analytics_seed_context;

COMMIT;

SELECT
  keys.name,
  COUNT(logs.id) AS request_count,
  SUM(logs.total_tokens) AS total_tokens,
  ROUND(SUM(logs.total_cost), 2) AS total_cost
FROM usage_logs AS logs
JOIN api_keys AS keys
  ON keys.id = logs.api_key_id
WHERE keys.key LIKE 'ah_api_key_analytics_seed_%'
GROUP BY keys.id, keys.name
ORDER BY total_cost DESC;
