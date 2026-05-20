CREATE UNIQUE INDEX IF NOT EXISTS users_email_unique ON users (email);

CREATE INDEX IF NOT EXISTS "IDX_AIUP" ON auth_identity (user_id, provider_id);

CREATE INDEX IF NOT EXISTS "IDX_CTS" ON categories (team, sort, id);
CREATE UNIQUE INDEX IF NOT EXISTS "IDX_CTN" ON categories (team, name);

CREATE INDEX IF NOT EXISTS "IDX_ETTH" ON enroll_tokens (token_hash);

CREATE INDEX IF NOT EXISTS "IDX_KTI" ON keys (team_id, id DESC);

CREATE INDEX IF NOT EXISTS "IDX_MTUU" ON m_team_user (user_id, team_id);

CREATE INDEX IF NOT EXISTS "IDX_SIARD" ON server_info (end_time) WHERE auto_renew = TRUE AND cycle > 0 AND end_time IS NOT NULL;

CREATE INDEX IF NOT EXISTS "IDX_STMO" ON servers (team_id, allow_monitor, weight DESC, id DESC);
CREATE INDEX IF NOT EXISTS "IDX_STTO" ON servers (team_id, allow_terminal, weight DESC, id DESC);
CREATE INDEX IF NOT EXISTS "IDX_SC" ON servers (category);

CREATE INDEX IF NOT EXISTS "IDX_TOI" ON teams (owner_id);

CREATE INDEX IF NOT EXISTS "IDX_UCAT" ON users_config (active_team);
