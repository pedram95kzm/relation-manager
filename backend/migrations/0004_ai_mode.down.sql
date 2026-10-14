-- Refuse a rollback that would erase or mislabel local example history.
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM provider_config WHERE provider = 'mock')
       OR EXISTS (SELECT 1 FROM analyses WHERE provider = 'mock')
       OR EXISTS (SELECT 1 FROM usage_events WHERE provider = 'mock') THEN
        RAISE EXCEPTION 'Cannot roll back local AI mode while mock provider records exist';
    END IF;
END $$;
ALTER TABLE usage_events DROP CONSTRAINT usage_events_provider_check;
ALTER TABLE usage_events ADD CONSTRAINT usage_events_provider_check
    CHECK (provider IN ('', 'openai', 'deepseek', 'anthropic', 'gemini'));
ALTER TABLE analyses DROP CONSTRAINT analyses_provider_check;
ALTER TABLE analyses ADD CONSTRAINT analyses_provider_check
    CHECK (provider IN ('', 'openai', 'deepseek', 'anthropic', 'gemini'));
ALTER TABLE provider_config DROP CONSTRAINT provider_config_provider_check;
ALTER TABLE provider_config ADD CONSTRAINT provider_config_provider_check
    CHECK (provider IN ('openai', 'deepseek', 'anthropic', 'gemini'));
ALTER TABLE provider_config DROP CONSTRAINT provider_config_mock_key_check;
ALTER TABLE provider_config DROP CONSTRAINT provider_config_credential_check;
ALTER TABLE provider_config ADD CONSTRAINT provider_config_check
    CHECK (NOT active OR encrypted_key IS NOT NULL);
