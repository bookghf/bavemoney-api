-- Recurring rules: templates (rent, salary, monthly transfers between the
-- user's own accounts) that the API turns into transactions when they fall
-- due. next_run_on is the next local day (in time_zone) to create; the runner
-- advances it in the same database transaction that inserts the transactions.
CREATE TABLE IF NOT EXISTS recurring_rules (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type          TEXT NOT NULL CHECK (type IN ('income', 'expense', 'transfer')),
    -- A rule goes with its accounts: accounts are only hard-deleted by an
    -- account reset, which must not be blocked by a rule.
    account_id    UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    to_account_id UUID REFERENCES accounts(id) ON DELETE CASCADE,
    -- Like transactions, a deleted category leaves the rule uncategorized.
    category_id   UUID REFERENCES categories(id) ON DELETE SET NULL,
    amount        NUMERIC(18,2) NOT NULL CHECK (amount > 0),
    currency      CHAR(3) NOT NULL REFERENCES currencies(code),
    note          TEXT,
    frequency     TEXT NOT NULL CHECK (frequency IN ('monthly', 'weekly')),
    -- day_of_month 29-31 runs on the last day of shorter months.
    day_of_month  SMALLINT CHECK (day_of_month BETWEEN 1 AND 31),
    -- weekday: 0 = Sunday ... 6 = Saturday.
    weekday       SMALLINT CHECK (weekday BETWEEN 0 AND 6),
    start_date    DATE NOT NULL,
    end_date      DATE,
    next_run_on   DATE NOT NULL,
    last_run_on   DATE,
    time_zone     TEXT NOT NULL DEFAULT 'UTC',
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    -- Why the runner paused the rule (e.g. account_archived); NULL when the
    -- user paused it or it is active.
    pause_reason  TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_recurring_schedule CHECK (
        (frequency = 'monthly' AND day_of_month IS NOT NULL AND weekday IS NULL)
        OR (frequency = 'weekly' AND weekday IS NOT NULL AND day_of_month IS NULL)),
    CONSTRAINT chk_recurring_transfer_target CHECK (
        (type = 'transfer') = (to_account_id IS NOT NULL)
        AND to_account_id IS DISTINCT FROM account_id),
    CONSTRAINT chk_recurring_end_date CHECK (end_date IS NULL OR end_date >= start_date)
);

CREATE INDEX IF NOT EXISTS idx_recurring_rules_user ON recurring_rules (user_id);
-- The runner's scan for due rules.
CREATE INDEX IF NOT EXISTS idx_recurring_rules_due ON recurring_rules (next_run_on) WHERE is_active;
