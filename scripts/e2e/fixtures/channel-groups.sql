-- Reusable SQLite fixture for the channel list provider tabs.
-- Covers provider grouping of protocol variants and the image-generation split
-- driven by settings.primaryApiFormat. Run only against an isolated test
-- database. Re-running replaces rows marked with the fixed Channel Groups Seed
-- prefix and leaves unrelated rows intact.
--
-- Expected tabs (enabled + disabled, archived excluded), sorted by count then key:
--   OpenAI 6 | DeepSeek 5 | Moonshot 4 | OpenAI · Image 3 | OpenCode Go 2
--   | GitHub 1 | GitHub Copilot 1 | Ollama 1

PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;

BEGIN IMMEDIATE;

CREATE TEMP TABLE channel_groups_seed_targets AS
SELECT id FROM channels WHERE name LIKE '[Channel Groups Seed] %';

CREATE TEMP TABLE channel_groups_seed_requests AS
SELECT id FROM requests WHERE channel_id IN (SELECT id FROM channel_groups_seed_targets);

DELETE FROM usage_logs
WHERE channel_id IN (SELECT id FROM channel_groups_seed_targets)
   OR request_id IN (SELECT id FROM channel_groups_seed_requests);

DELETE FROM request_executions
WHERE channel_id IN (SELECT id FROM channel_groups_seed_targets)
   OR request_id IN (SELECT id FROM channel_groups_seed_requests);

DELETE FROM requests
WHERE id IN (SELECT id FROM channel_groups_seed_requests);

DELETE FROM channel_probes
WHERE channel_id IN (SELECT id FROM channel_groups_seed_targets);

DELETE FROM channel_model_price_versions
WHERE channel_model_price_id IN (
  SELECT id FROM channel_model_prices
  WHERE channel_id IN (SELECT id FROM channel_groups_seed_targets)
);

DELETE FROM channel_model_prices
WHERE channel_id IN (SELECT id FROM channel_groups_seed_targets);

DELETE FROM provider_quota_status
WHERE channel_id IN (SELECT id FROM channel_groups_seed_targets);

DELETE FROM channels
WHERE id IN (SELECT id FROM channel_groups_seed_targets);

DROP TABLE channel_groups_seed_requests;
DROP TABLE channel_groups_seed_targets;

-- settings column variants:
--   NULL                      legacy row created before settings existed
--   {"modelMappings":[]}      row saved by the dialog without the image option
--   primaryApiFormat = chat   explicit non-image value, exercises the NEQ branch
--   primaryApiFormat = image  the image-generation split
CREATE TEMP TABLE channel_groups_seed_channels (
  type TEXT NOT NULL,
  name TEXT NOT NULL,
  status TEXT NOT NULL,
  base_url TEXT NOT NULL,
  settings TEXT,
  supported_models TEXT NOT NULL,
  default_test_model TEXT NOT NULL
);

INSERT INTO channel_groups_seed_channels VALUES
  -- OpenAI chat group (6): openai + openai_responses fold into one tab
  ('openai', 'OpenAI Chat Primary', 'enabled', 'https://api.openai.com/v1',
   NULL, '["gpt-4o","gpt-4o-mini"]', 'gpt-4o-mini'),
  ('openai', 'OpenAI Chat Secondary', 'enabled', 'https://api.openai.com/v1',
   '{"modelMappings":[]}', '["gpt-4o","gpt-4o-mini"]', 'gpt-4o-mini'),
  ('openai', 'OpenAI Chat Explicit', 'enabled', 'https://api.openai.com/v1',
   '{"modelMappings":[],"primaryApiFormat":"openai/chat_completions"}', '["gpt-4o"]', 'gpt-4o'),
  ('openai', 'OpenAI Chat Disabled', 'disabled', 'https://api.openai.com/v1',
   '{"modelMappings":[]}', '["gpt-4o"]', 'gpt-4o'),
  ('openai_responses', 'OpenAI Responses A', 'enabled', 'https://api.openai.com/v1',
   '{"modelMappings":[]}', '["gpt-5"]', 'gpt-5'),
  ('openai_responses', 'OpenAI Responses B', 'enabled', 'https://api.openai.com/v1',
   NULL, '["gpt-5"]', 'gpt-5'),

  -- OpenAI image group (3): same channel type, split by primaryApiFormat
  ('openai', 'OpenAI Image Main', 'enabled', 'https://api.openai.com/v1',
   '{"modelMappings":[],"primaryApiFormat":"openai/image_generation"}', '["gpt-image-1"]', 'gpt-image-1'),
  ('openai', 'OpenAI Image Backup', 'enabled', 'https://api.openai.com/v1',
   '{"modelMappings":[],"primaryApiFormat":"openai/image_generation"}', '["gpt-image-1"]', 'gpt-image-1'),
  ('openai', 'OpenAI Image Disabled', 'disabled', 'https://api.openai.com/v1',
   '{"modelMappings":[],"primaryApiFormat":"openai/image_generation"}', '["gpt-image-1"]', 'gpt-image-1'),

  -- DeepSeek group (5): deepseek + deepseek_anthropic
  ('deepseek', 'DeepSeek Chat A', 'enabled', 'https://api.deepseek.com/v1',
   '{"modelMappings":[]}', '["deepseek-chat"]', 'deepseek-chat'),
  ('deepseek', 'DeepSeek Chat B', 'enabled', 'https://api.deepseek.com/v1',
   NULL, '["deepseek-chat"]', 'deepseek-chat'),
  ('deepseek', 'DeepSeek Chat Disabled', 'disabled', 'https://api.deepseek.com/v1',
   '{"modelMappings":[]}', '["deepseek-chat"]', 'deepseek-chat'),
  ('deepseek_anthropic', 'DeepSeek Anthropic A', 'enabled', 'https://api.deepseek.com/anthropic',
   '{"modelMappings":[]}', '["deepseek-chat"]', 'deepseek-chat'),
  ('deepseek_anthropic', 'DeepSeek Anthropic B', 'enabled', 'https://api.deepseek.com/anthropic',
   NULL, '["deepseek-chat"]', 'deepseek-chat'),

  -- Moonshot group (4): moonshot + moonshot_anthropic + moonshot_coding
  ('moonshot', 'Moonshot Chat A', 'enabled', 'https://api.moonshot.cn/v1',
   '{"modelMappings":[]}', '["kimi-k2-0905-preview"]', 'kimi-k2-0905-preview'),
  ('moonshot', 'Moonshot Chat B', 'enabled', 'https://api.moonshot.cn/v1',
   NULL, '["kimi-k2-0905-preview"]', 'kimi-k2-0905-preview'),
  ('moonshot_anthropic', 'Moonshot Anthropic', 'enabled', 'https://api.moonshot.cn/anthropic',
   '{"modelMappings":[]}', '["kimi-k2-0905-preview"]', 'kimi-k2-0905-preview'),
  ('moonshot_coding', 'Moonshot Kimi Coding', 'enabled', 'https://api.kimi.com/coding',
   '{"modelMappings":[]}', '["kimi-for-coding"]', 'kimi-for-coding'),

  -- OpenCode Go group (2): the bare prefix `opencode` is not a channel type,
  -- so the old prefix grouping showed these as two tabs
  ('opencode_go', 'OpenCode Go Chat', 'enabled', 'https://opencode.ai/zen/go',
   '{"modelMappings":[]}', '["gpt-5"]', 'gpt-5'),
  ('opencode_go_anthropic', 'OpenCode Go Anthropic', 'enabled', 'https://opencode.ai/zen/go',
   '{"modelMappings":[]}', '["claude-sonnet-4-5"]', 'claude-sonnet-4-5'),

  -- GitHub vs GitHub Copilot (1 each): different providers that the old prefix
  -- grouping wrongly folded together
  ('github', 'GitHub Models', 'enabled', 'https://models.github.ai/inference',
   '{"modelMappings":[]}', '["openai/gpt-4o"]', 'openai/gpt-4o'),
  ('github_copilot', 'GitHub Copilot', 'enabled', 'https://api.githubcopilot.com',
   '{"modelMappings":[]}', '["gpt-4o"]', 'gpt-4o'),

  -- Ollama (1): only the Anthropic variant exists; relies on the
  -- ollama_anthropic -> ollama mapping to land in the Ollama tab
  ('ollama_anthropic', 'Ollama Anthropic Only', 'enabled', 'https://ollama.com',
   '{"modelMappings":[]}', '["gpt-oss:120b"]', 'gpt-oss:120b'),

  -- Archived rows are excluded from tab counts by default
  ('openai', 'OpenAI Archived', 'archived', 'https://api.openai.com/v1',
   NULL, '["gpt-4o"]', 'gpt-4o');

INSERT INTO channels (
  type,
  name,
  status,
  base_url,
  credentials,
  supported_models,
  tags,
  default_test_model,
  settings
)
SELECT
  type,
  '[Channel Groups Seed] ' || name,
  status,
  base_url,
  json_object('apiKey', 'channel-groups-seed-' || lower(replace(name, ' ', '-'))),
  json(supported_models),
  json('["channel-groups-seed"]'),
  default_test_model,
  CASE WHEN settings IS NULL THEN NULL ELSE json(settings) END
FROM channel_groups_seed_channels;

DROP TABLE channel_groups_seed_channels;

COMMIT;

-- Mirrors countChannelsByType: type x image flag, archived excluded.
SELECT
  type,
  CASE
    WHEN json_extract(settings, '$.primaryApiFormat') = 'openai/image_generation'
    THEN 'openai/image_generation'
    ELSE ''
  END AS primary_api_format,
  COUNT(*) AS channel_count
FROM channels
WHERE name LIKE '[Channel Groups Seed] %'
  AND deleted_at = 0
  AND status != 'archived'
GROUP BY type, primary_api_format
ORDER BY channel_count DESC, type, primary_api_format;
