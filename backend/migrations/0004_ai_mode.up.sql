-- Local example mode uses the existing provider lifecycle without credentials.
ALTER TABLE provider_config DROP CONSTRAINT provider_config_provider_check;
ALTER TABLE provider_config ADD CONSTRAINT provider_config_provider_check
    CHECK (provider IN ('openai', 'deepseek', 'anthropic', 'gemini', 'mock'));
ALTER TABLE provider_config DROP CONSTRAINT provider_config_check;
ALTER TABLE provider_config ADD CONSTRAINT provider_config_credential_check
    CHECK (provider = 'mock' OR NOT active OR encrypted_key IS NOT NULL);
ALTER TABLE provider_config ADD CONSTRAINT provider_config_mock_key_check
    CHECK (provider <> 'mock' OR encrypted_key IS NULL);
ALTER TABLE analyses DROP CONSTRAINT analyses_provider_check;
ALTER TABLE analyses ADD CONSTRAINT analyses_provider_check
    CHECK (provider IN ('', 'openai', 'deepseek', 'anthropic', 'gemini', 'mock'));
ALTER TABLE usage_events DROP CONSTRAINT usage_events_provider_check;
ALTER TABLE usage_events ADD CONSTRAINT usage_events_provider_check
    CHECK (provider IN ('', 'openai', 'deepseek', 'anthropic', 'gemini', 'mock'));
