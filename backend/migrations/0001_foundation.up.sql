CREATE TABLE IF NOT EXISTS app_settings (
 id integer PRIMARY KEY CHECK (id = 1),
 global_daily integer NOT NULL CHECK (global_daily > 0),
 user_daily integer NOT NULL CHECK (user_daily > 0),
 updated_at timestamptz NOT NULL DEFAULT now()
);
