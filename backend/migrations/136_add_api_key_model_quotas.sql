-- Add per-model quota fields to api_keys table

ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS model_quota_limits JSONB;
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS model_quota_used JSONB;

COMMENT ON COLUMN api_keys.model_quota_limits IS 'Per-model quota limits in USD keyed by requested model';
COMMENT ON COLUMN api_keys.model_quota_used IS 'Per-model used quota in USD keyed by requested model';
