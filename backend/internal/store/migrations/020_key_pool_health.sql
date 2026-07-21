-- 020_key_pool_health.sql
-- Provider-aware cooldown and health diagnostics for pooled API keys.

ALTER TABLE api_key_pool ADD COLUMN cooldown_until TEXT;
ALTER TABLE api_key_pool ADD COLUMN last_error TEXT;
ALTER TABLE api_key_pool ADD COLUMN last_status TEXT;

CREATE INDEX IF NOT EXISTS idx_api_key_pool_lease
    ON api_key_pool(provider, is_active, cooldown_until, last_used_at);
