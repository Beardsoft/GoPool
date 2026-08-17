-- Indexes for hot read paths. These were full scans that degrade as the tables grow.
CREATE INDEX IF NOT EXISTS idx_stakers_address ON stakers(address, epoch_number);
CREATE INDEX IF NOT EXISTS idx_payslips_address ON payslips(address);
CREATE INDEX IF NOT EXISTS idx_payslips_status ON payslips(status);
CREATE INDEX IF NOT EXISTS idx_payslips_tx_hash ON payslips(tx_hash);
CREATE INDEX IF NOT EXISTS idx_rewards_epoch ON rewards(epoch_number);
CREATE INDEX IF NOT EXISTS idx_audit_logs_status_address ON audit_logs(status, address);
CREATE INDEX IF NOT EXISTS idx_transactions_address ON transactions(address);
