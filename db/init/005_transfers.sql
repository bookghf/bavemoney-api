-- A transfer is one transactions row with type 'transfer': it moves amount out
-- of account_id and into to_account_id. Both accounts must share a currency, so
-- the same amount applies to each side.
ALTER TABLE transactions ADD COLUMN IF NOT EXISTS to_account_id UUID REFERENCES accounts(id);

-- Balances and the account filter look transfers up from the receiving side.
CREATE INDEX IF NOT EXISTS idx_transactions_to_account
    ON transactions (to_account_id)
    WHERE to_account_id IS NOT NULL;
