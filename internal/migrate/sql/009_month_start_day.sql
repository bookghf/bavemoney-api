-- "My month starts on payday": the day of the month (1-28) a user's month
-- begins on. Reports for period=month and monthly budgets follow it; 1 keeps
-- calendar months. Capped at 28 so every month has that day.
ALTER TABLE users ADD COLUMN IF NOT EXISTS month_start_day SMALLINT NOT NULL DEFAULT 1;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_users_month_start_day') THEN
        ALTER TABLE users ADD CONSTRAINT chk_users_month_start_day
            CHECK (month_start_day BETWEEN 1 AND 28);
    END IF;
END $$;
