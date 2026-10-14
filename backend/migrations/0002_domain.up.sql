CREATE TABLE users (
    id uuid PRIMARY KEY,
    email varchar(254) NOT NULL UNIQUE CHECK (email = lower(btrim(email)) AND email <> ''),
    name varchar(120) NOT NULL DEFAULT '',
    password_hash varchar(255) NOT NULL DEFAULT '',
    role varchar(16) NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    age integer CHECK (age BETWEEN 18 AND 120),
    gender varchar(32) NOT NULL DEFAULT '' CHECK (gender IN ('', 'female', 'male', 'nonbinary', 'other', 'prefer_not_to_say')),
    disabled boolean NOT NULL DEFAULT false,
    daily_limit integer CHECK (daily_limit BETWEEN 1 AND 10000),
    auth_version bigint NOT NULL DEFAULT 1 CHECK (auth_version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX users_created_idx ON users (created_at DESC, id);

CREATE TABLE auth_identities (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider varchar(32) NOT NULL CHECK (provider = 'google'),
    subject varchar(255) NOT NULL CHECK (subject <> ''),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, subject),
    UNIQUE (user_id, provider)
);

CREATE TABLE refresh_sessions (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    family_id uuid NOT NULL,
    token_hash varchar(64) NOT NULL UNIQUE CHECK (token_hash ~ '^[0-9a-f]{64}$'),
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (expires_at > created_at)
);
CREATE INDEX refresh_sessions_user_idx ON refresh_sessions(user_id);
CREATE INDEX refresh_sessions_family_idx ON refresh_sessions(family_id);
CREATE INDEX refresh_sessions_expiry_idx ON refresh_sessions(expires_at);

CREATE TABLE oauth_states (
    state_hash varchar(64) PRIMARY KEY CHECK (state_hash ~ '^[0-9a-f]{64}$'),
    verifier varchar(128) NOT NULL CHECK (length(verifier) BETWEEN 43 AND 128),
    nonce_hash varchar(64) NOT NULL CHECK (nonce_hash ~ '^[0-9a-f]{64}$'),
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (expires_at > created_at)
);
CREATE INDEX oauth_states_expiry_idx ON oauth_states(expires_at);

CREATE TABLE questionnaires (
    module varchar(32) NOT NULL CHECK (module IN ('self', 'compatibility', 'conflict')),
    version varchar(64) NOT NULL CHECK (version <> ''),
    scoring_version varchar(64) NOT NULL CHECK (scoring_version <> ''),
    definition jsonb NOT NULL CHECK (jsonb_typeof(definition) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (module, version),
    UNIQUE (module, version, scoring_version)
);

CREATE TABLE analyses (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    module varchar(32) NOT NULL CHECK (module IN ('self', 'compatibility', 'conflict')),
    locale varchar(2) NOT NULL CHECK (locale IN ('fa', 'en')),
    questionnaire_version varchar(64) NOT NULL,
    scoring_version varchar(64) NOT NULL,
    status varchar(24) NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'processing', 'completed', 'failed', 'insufficient')),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    answers jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(answers) = 'object'),
    context jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(context) = 'object'),
    scores jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(scores) = 'object'),
    report jsonb CHECK (report IS NULL OR jsonb_typeof(report) = 'object'),
    consent_at timestamptz,
    prompt_version varchar(64) NOT NULL DEFAULT '',
    provider varchar(32) NOT NULL DEFAULT '' CHECK (provider IN ('', 'openai', 'deepseek', 'anthropic', 'gemini')),
    model varchar(128) NOT NULL DEFAULT '',
    report_schema varchar(32) NOT NULL DEFAULT '',
    failure_code varchar(64) NOT NULL DEFAULT '',
    processing_started_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (module, questionnaire_version, scoring_version) REFERENCES questionnaires(module, version, scoring_version) ON DELETE RESTRICT,
    CHECK (status <> 'completed' OR report IS NOT NULL),
    CHECK (status NOT IN ('processing', 'completed') OR consent_at IS NOT NULL)
);
CREATE INDEX analyses_owner_history_idx ON analyses(user_id, created_at DESC, id);
CREATE INDEX analyses_activity_idx ON analyses(created_at, module, status);
CREATE INDEX analyses_processing_idx ON analyses(processing_started_at) WHERE status = 'processing';

-- Only sanitized operational metadata survives deletion of sensitive records.
CREATE TABLE usage_events (
    id uuid PRIMARY KEY,
    user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    analysis_id uuid REFERENCES analyses(id) ON DELETE SET NULL,
    provider varchar(32) NOT NULL DEFAULT '' CHECK (provider IN ('', 'openai', 'deepseek', 'anthropic', 'gemini')),
    model varchar(128) NOT NULL DEFAULT '',
    status varchar(24) NOT NULL CHECK (status IN ('success', 'error', 'rejected')),
    error_code varchar(64) NOT NULL DEFAULT '',
    latency_ms bigint NOT NULL DEFAULT 0 CHECK (latency_ms >= 0),
    input_tokens bigint NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    output_tokens bigint NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX usage_events_created_idx ON usage_events(created_at DESC, id);
CREATE INDEX usage_events_user_idx ON usage_events(user_id, created_at DESC);
CREATE INDEX usage_events_provider_idx ON usage_events(provider, created_at);
CREATE INDEX usage_events_analysis_idx ON usage_events(analysis_id);

-- UTC dates and scope keys support row-locked, atomic quota reservations.
CREATE TABLE daily_usage (
    day date NOT NULL,
    scope_key varchar(128) NOT NULL CHECK (scope_key <> ''),
    counter bigint NOT NULL DEFAULT 0 CHECK (counter >= 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (day, scope_key)
);

CREATE TABLE provider_config (
    id integer PRIMARY KEY CHECK (id = 1),
    provider varchar(32) NOT NULL CHECK (provider IN ('openai', 'deepseek', 'anthropic', 'gemini')),
    model varchar(128) NOT NULL CHECK (model <> ''),
    encrypted_key bytea,
    max_tokens integer NOT NULL DEFAULT 4096 CHECK (max_tokens BETWEEN 512 AND 16000),
    timeout_seconds integer NOT NULL DEFAULT 60 CHECK (timeout_seconds BETWEEN 1 AND 120),
    retries integer NOT NULL DEFAULT 1 CHECK (retries BETWEEN 0 AND 3),
    active boolean NOT NULL DEFAULT false,
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (NOT active OR encrypted_key IS NOT NULL)
);

CREATE TABLE audit_events (
    id uuid PRIMARY KEY,
    actor_id uuid REFERENCES users(id) ON DELETE SET NULL,
    target_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    action varchar(64) NOT NULL CHECK (action <> ''),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_events_created_idx ON audit_events(created_at DESC, id);
CREATE INDEX audit_events_actor_idx ON audit_events(actor_id, created_at DESC);
CREATE INDEX audit_events_target_idx ON audit_events(target_user_id, created_at DESC);
