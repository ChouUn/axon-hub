-- Reusable SQLite fixture for the model analytics dashboard.
-- Run only against an isolated test database. Re-running replaces rows marked
-- with the fixed Model Analytics Seed prefixes and leaves unrelated rows intact.
-- Data is assigned to the active project with the smallest ID.

PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;

BEGIN IMMEDIATE;

CREATE TEMP TABLE model_analytics_seed_context (
  project_id INTEGER NOT NULL
);

INSERT INTO model_analytics_seed_context (project_id)
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
  WHERE external_id LIKE 'model-analytics-seed:%'
);

DELETE FROM request_executions
WHERE request_id IN (
  SELECT id
  FROM requests
  WHERE external_id LIKE 'model-analytics-seed:%'
);

DELETE FROM requests
WHERE external_id LIKE 'model-analytics-seed:%';

DELETE FROM channels
WHERE name LIKE '[Model Analytics Seed] %';

CREATE TEMP TABLE model_analytics_seed_channels (
  suffix TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  tags TEXT NOT NULL
);

INSERT INTO model_analytics_seed_channels VALUES
  ('premium-east', 'Premium East', json('["premium","east"]')),
  ('budget-west', 'Budget West', json('["budget","west"]')),
  ('premium-backup', 'Premium Backup', json('["premium","backup"]'));

INSERT INTO channels (
  type,
  name,
  status,
  credentials,
  supported_models,
  tags,
  default_test_model
)
SELECT
  'openai',
  '[Model Analytics Seed] ' || name,
  'enabled',
  json_object('apiKey', 'model-analytics-seed-' || suffix),
  json('["gpt-model-mock","claude-model-mock"]'),
  tags,
  'gpt-model-mock'
FROM model_analytics_seed_channels;

CREATE TEMP TABLE model_analytics_seed_requests (
  suffix TEXT PRIMARY KEY,
  model_id TEXT NOT NULL,
  day_offset INTEGER NOT NULL,
  hour INTEGER NOT NULL,
  final_channel_suffix TEXT,
  prompt_tokens INTEGER NOT NULL,
  prompt_cached_tokens INTEGER NOT NULL,
  completion_tokens INTEGER NOT NULL,
  total_cost REAL,
  stream INTEGER NOT NULL,
  latency_ms INTEGER,
  first_token_latency_ms INTEGER
);

INSERT INTO model_analytics_seed_requests VALUES
  ('gpt-same-channel-retry', 'gpt-model-mock', 0, 10,
   'premium-east', 2700, 900, 300, 6.0, 1, 2000, 500),
  ('gpt-cross-channel-retry', 'gpt-model-mock', 0, 11,
   'premium-backup', 900, 450, 100, 2.0, 0, 1000, NULL),
  ('gpt-budget-success', 'gpt-model-mock', -1, 12,
   'budget-west', 3600, 720, 400, 4.0, 1, 3000, 1000),
  ('gpt-total-failure', 'gpt-model-mock', 0, 13,
   NULL, 0, 0, 0, NULL, 0, NULL, NULL),
  ('claude-premium-success', 'claude-model-mock', 0, 14,
   'premium-east', 4500, 2250, 500, 15.0, 1, 2500, 500),
  ('claude-budget-success', 'claude-model-mock', -1, 15,
   'budget-west', 1800, 180, 200, 5.0, 0, 1000, NULL);

CREATE TEMP VIEW model_analytics_seed_request_data AS
SELECT
  requests.*,
  datetime(
    date('now', printf('%+d days', requests.day_offset)),
    printf('+%d hours', requests.hour)
  ) AS created_at
FROM model_analytics_seed_requests AS requests;

INSERT INTO requests (
  created_at,
  updated_at,
  source,
  model_id,
  request_body,
  external_id,
  status,
  stream,
  channel_id,
  project_id
)
SELECT
  rows.created_at,
  rows.created_at,
  'test',
  rows.model_id,
  json('{"seed":true}'),
  'model-analytics-seed:' || rows.suffix,
  CASE WHEN rows.final_channel_suffix IS NULL THEN 'failed' ELSE 'completed' END,
  rows.stream,
  channels.id,
  context.project_id
FROM model_analytics_seed_request_data AS rows
CROSS JOIN model_analytics_seed_context AS context
LEFT JOIN model_analytics_seed_channels AS seed_channels
  ON seed_channels.suffix = rows.final_channel_suffix
LEFT JOIN channels
  ON channels.name = '[Model Analytics Seed] ' || seed_channels.name
  AND channels.deleted_at = 0;

CREATE TEMP TABLE model_analytics_seed_executions (
  suffix TEXT PRIMARY KEY,
  request_suffix TEXT NOT NULL,
  attempt_order INTEGER NOT NULL,
  channel_suffix TEXT NOT NULL,
  status TEXT NOT NULL,
  latency_ms INTEGER,
  stream INTEGER NOT NULL,
  first_token_latency_ms INTEGER
);

INSERT INTO model_analytics_seed_executions VALUES
  ('gpt-same-channel-failed', 'gpt-same-channel-retry', 1,
   'premium-east', 'failed', 700, 1, 300),
  ('gpt-same-channel-completed', 'gpt-same-channel-retry', 2,
   'premium-east', 'completed', 2000, 1, 500),
  ('gpt-cross-channel-failed', 'gpt-cross-channel-retry', 1,
   'budget-west', 'failed', 800, 0, NULL),
  ('gpt-cross-channel-completed', 'gpt-cross-channel-retry', 2,
   'premium-backup', 'completed', 1000, 0, NULL),
  ('gpt-budget-completed', 'gpt-budget-success', 1,
   'budget-west', 'completed', 3000, 1, 1000),
  ('gpt-total-failed-premium', 'gpt-total-failure', 1,
   'premium-east', 'failed', 600, 0, NULL),
  ('gpt-total-failed-budget', 'gpt-total-failure', 2,
   'budget-west', 'failed', 500, 0, NULL),
  ('claude-premium-completed', 'claude-premium-success', 1,
   'premium-east', 'completed', 2500, 1, 500),
  ('claude-budget-failed', 'claude-budget-success', 1,
   'premium-backup', 'failed', 400, 0, NULL),
  ('claude-budget-completed', 'claude-budget-success', 2,
   'budget-west', 'completed', 1000, 0, NULL);

INSERT INTO request_executions (
  created_at,
  updated_at,
  project_id,
  model_id,
  request_body,
  status,
  stream,
  metrics_latency_ms,
  metrics_first_token_latency_ms,
  channel_id,
  request_id
)
SELECT
  datetime(requests.created_at, printf('+%d seconds', row_number() OVER (
    PARTITION BY execution_rows.request_suffix
    ORDER BY execution_rows.attempt_order
  ))),
  requests.created_at,
  context.project_id,
  request_rows.model_id,
  json('{"seed":true}'),
  execution_rows.status,
  execution_rows.stream,
  execution_rows.latency_ms,
  execution_rows.first_token_latency_ms,
  channels.id,
  requests.id
FROM model_analytics_seed_executions AS execution_rows
JOIN requests
  ON requests.external_id =
    'model-analytics-seed:' || execution_rows.request_suffix
JOIN model_analytics_seed_request_data AS request_rows
  ON request_rows.suffix = execution_rows.request_suffix
JOIN model_analytics_seed_channels AS seed_channels
  ON seed_channels.suffix = execution_rows.channel_suffix
JOIN channels
  ON channels.name = '[Model Analytics Seed] ' || seed_channels.name
  AND channels.deleted_at = 0
CROSS JOIN model_analytics_seed_context AS context;

INSERT INTO usage_logs (
  created_at,
  updated_at,
  model_id,
  prompt_tokens,
  prompt_cached_tokens,
  completion_tokens,
  total_tokens,
  source,
  total_cost,
  cost_items,
  channel_id,
  project_id,
  request_id
)
SELECT
  rows.created_at,
  rows.created_at,
  rows.model_id,
  rows.prompt_tokens,
  rows.prompt_cached_tokens,
  rows.completion_tokens,
  rows.prompt_tokens + rows.completion_tokens,
  'test',
  rows.total_cost,
  json('[]'),
  channels.id,
  context.project_id,
  requests.id
FROM model_analytics_seed_request_data AS rows
JOIN requests
  ON requests.external_id = 'model-analytics-seed:' || rows.suffix
JOIN model_analytics_seed_channels AS seed_channels
  ON seed_channels.suffix = rows.final_channel_suffix
JOIN channels
  ON channels.name = '[Model Analytics Seed] ' || seed_channels.name
  AND channels.deleted_at = 0
CROSS JOIN model_analytics_seed_context AS context
WHERE rows.final_channel_suffix IS NOT NULL;

DROP TABLE model_analytics_seed_executions;
DROP VIEW model_analytics_seed_request_data;
DROP TABLE model_analytics_seed_requests;
DROP TABLE model_analytics_seed_channels;
DROP TABLE model_analytics_seed_context;

COMMIT;

SELECT
  projects.id AS project_id,
  projects.name AS project_name,
  executions.model_id,
  channels.name AS channel_name,
  COUNT(executions.id) AS request_count,
  SUM(CASE WHEN executions.status = 'completed' THEN 1 ELSE 0 END) AS successes
FROM request_executions AS executions
JOIN channels ON channels.id = executions.channel_id
JOIN requests ON requests.id = executions.request_id
JOIN projects ON projects.id = executions.project_id
WHERE requests.external_id LIKE 'model-analytics-seed:%'
GROUP BY
  projects.id,
  projects.name,
  executions.model_id,
  channels.id,
  channels.name
ORDER BY projects.id, executions.model_id, channel_name;
