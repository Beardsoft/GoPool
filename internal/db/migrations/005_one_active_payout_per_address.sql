CREATE UNIQUE INDEX IF NOT EXISTS idx_transactions_one_awaiting_per_address
ON transactions(address)
WHERE status = 'awaiting_confirmation';
