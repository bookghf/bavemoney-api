-- Integrity, security, and performance fixes from the 2026-10 review.

-- Refresh token families: every rotation stays in its login's family, so a
-- replayed (already rotated) token can revoke the whole family.
ALTER TABLE refresh_tokens ADD COLUMN IF NOT EXISTS family_id UUID NOT NULL DEFAULT gen_random_uuid();
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_family ON refresh_tokens (family_id) WHERE revoked_at IS NULL;

-- Emails are case-insensitive. Lowercase existing rows where that does not
-- collide with another account; lookups use lower(email) from now on.
UPDATE users u SET email = lower(u.email)
WHERE u.email <> lower(u.email)
  AND NOT EXISTS (SELECT 1 FROM users o WHERE o.email = lower(u.email));
CREATE INDEX IF NOT EXISTS idx_users_lower_email ON users (lower(email));

-- New users default to Thai baht, the app's primary market.
ALTER TABLE users ALTER COLUMN default_currency SET DEFAULT 'THB';

-- Hot query paths.
CREATE INDEX IF NOT EXISTS idx_transactions_user_occurred ON transactions (user_id, occurred_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_transactions_account ON transactions (account_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_transactions_category ON transactions (category_id) WHERE category_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_accounts_user ON accounts (user_id);
CREATE INDEX IF NOT EXISTS idx_categories_user ON categories (user_id);
CREATE INDEX IF NOT EXISTS idx_categories_parent ON categories (parent_id);
CREATE INDEX IF NOT EXISTS idx_budgets_user ON budgets (user_id);
CREATE INDEX IF NOT EXISTS idx_attachments_transaction ON attachments (transaction_id);
CREATE INDEX IF NOT EXISTS idx_exchange_rates_lookup ON exchange_rates (base_currency, target_currency, effective_date DESC);

-- Database-level guards for money rows. NOT VALID enforces them on every new
-- or updated row without failing on legacy rows written before the checks.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_transactions_type') THEN
        ALTER TABLE transactions ADD CONSTRAINT chk_transactions_type
            CHECK (type IN ('income', 'expense', 'transfer')) NOT VALID;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_transactions_amount_positive') THEN
        ALTER TABLE transactions ADD CONSTRAINT chk_transactions_amount_positive
            CHECK (amount > 0) NOT VALID;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_transactions_transfer_target') THEN
        ALTER TABLE transactions ADD CONSTRAINT chk_transactions_transfer_target
            CHECK ((type = 'transfer') = (to_account_id IS NOT NULL)
                   AND to_account_id IS DISTINCT FROM account_id) NOT VALID;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_accounts_type') THEN
        ALTER TABLE accounts ADD CONSTRAINT chk_accounts_type
            CHECK (type IN ('cash', 'bank', 'credit_card', 'e_wallet')) NOT VALID;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_accounts_name') THEN
        ALTER TABLE accounts ADD CONSTRAINT chk_accounts_name
            CHECK (length(btrim(name)) > 0) NOT VALID;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_budgets_amount_positive') THEN
        ALTER TABLE budgets ADD CONSTRAINT chk_budgets_amount_positive
            CHECK (amount > 0) NOT VALID;
    END IF;
END $$;

-- Deleting a category keeps its transactions (now uncategorized) and drops
-- budgets that tracked it, instead of failing with a foreign key error.
ALTER TABLE transactions DROP CONSTRAINT IF EXISTS transactions_category_id_fkey;
ALTER TABLE transactions ADD CONSTRAINT transactions_category_id_fkey
    FOREIGN KEY (category_id) REFERENCES categories(id) ON DELETE SET NULL;
ALTER TABLE budgets DROP CONSTRAINT IF EXISTS budgets_category_id_fkey;
ALTER TABLE budgets ADD CONSTRAINT budgets_category_id_fkey
    FOREIGN KEY (category_id) REFERENCES categories(id) ON DELETE CASCADE;

-- One row per push token: keep the newest registration of each duplicate.
DELETE FROM device_tokens d
USING device_tokens newer
WHERE d.fcm_token = newer.fcm_token AND d.created_at < newer.created_at;
DELETE FROM device_tokens d
USING device_tokens other
WHERE d.fcm_token = other.fcm_token AND d.created_at = other.created_at AND d.id < other.id;
CREATE UNIQUE INDEX IF NOT EXISTS idx_device_tokens_fcm_token ON device_tokens (fcm_token);
