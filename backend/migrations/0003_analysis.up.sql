ALTER TABLE analyses ADD COLUMN ai_consent_at timestamptz;

-- Only new provider attempts require the separately recorded sharing consent.
-- Existing completed records retain their historical consent representation.
ALTER TABLE analyses ADD CONSTRAINT analyses_processing_ai_consent
    CHECK (status <> 'processing' OR ai_consent_at IS NOT NULL);
