-- Forgot password: one active 6-digit code per user. Only a bcrypt hash is
-- stored; the code expires after 15 minutes and allows 5 tries.
CREATE TABLE IF NOT EXISTS password_reset_codes (
    user_id    UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    code_hash  TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    attempts   INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
