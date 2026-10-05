-- A color per account so accounts can be told apart at a glance. Like
-- categories.color it is a palette key ("orange", "violet", ...) the app maps
-- to light/dark shades; NULL means the app picks a default for the type.
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS color TEXT;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_accounts_color') THEN
        ALTER TABLE accounts ADD CONSTRAINT chk_accounts_color
            CHECK (color IS NULL OR color IN ('orange', 'amber', 'lime', 'cyan', 'indigo',
                                              'violet', 'fuchsia', 'pink', 'brown', 'slate'));
    END IF;
END $$;
