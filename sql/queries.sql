-- name: GetCursor :one
SELECT height FROM cursor WHERE name = ?;

-- name: UpsertCursor :exec
INSERT INTO cursor (name, height) VALUES (?, ?)
ON CONFLICT (name) DO UPDATE SET height = excluded.height;

-- name: InsertPolicyConstants :exec
INSERT INTO policy_constants (
    staking_contract_address, transaction_validity_window, blocks_per_batch,
    batches_per_epoch, blocks_per_epoch, validator_deposit, minimum_stake,
    total_supply, block_separation_time, jail_epochs, genesis_block_number
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetLatestPolicyConstants :one
SELECT staking_contract_address, transaction_validity_window, blocks_per_batch,
    batches_per_epoch, blocks_per_epoch, validator_deposit, minimum_stake,
    total_supply, block_separation_time, jail_epochs, genesis_block_number
FROM policy_constants ORDER BY id DESC LIMIT 1;

-- name: EpochExists :one
SELECT EXISTS(SELECT 1 FROM epochs WHERE number = ?);

-- name: InsertEpoch :exec
INSERT INTO epochs (number, num_stakers, balance, status) VALUES (?, ?, ?, ?);

-- name: GetEpochStatus :one
SELECT status, num_stakers FROM epochs WHERE number = ?;

-- name: SetEpochStatus :exec
UPDATE epochs SET status = ? WHERE number = ?;

-- name: FinalizeCompletedEpochs :many
UPDATE epochs SET status = 'completed' WHERE number IN (
    SELECT r.epoch_number
    FROM rewards AS r
    INNER JOIN (
        SELECT batch_number, COUNT(*) AS total, SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) AS done
        FROM payslips
        GROUP BY batch_number
    ) AS p ON p.batch_number = r.batch_number
    INNER JOIN epochs AS e ON e.number = r.epoch_number
    WHERE e.status = 'in_progress'
    GROUP BY r.epoch_number
    HAVING SUM(p.total) = SUM(p.done)
) RETURNING number;

-- name: InsertStaker :exec
INSERT INTO stakers (epoch_number, address, stake, percentage) VALUES (?, ?, ?, ?);

-- name: GetStakersForEpoch :many
SELECT address, stake, percentage FROM stakers WHERE epoch_number = ?;

-- name: RewardExists :one
SELECT EXISTS(SELECT 1 FROM rewards WHERE batch_number = ?);

-- name: InsertReward :exec
INSERT INTO rewards (batch_number, epoch_number, amount, pool_fee, num_stakers)
VALUES (?, ?, ?, ?, ?);

-- name: InsertPayslip :exec
INSERT INTO payslips (batch_number, address, amount, status) VALUES (?, ?, ?, ?);

-- name: GetEligibleForPayout :many
SELECT address, CAST(SUM(amount) AS INTEGER) AS total
FROM payslips
WHERE status = 'pending'
GROUP BY address
HAVING SUM(amount) >= ?;

-- name: MarkPayslipsOutForPayment :exec
UPDATE payslips SET status = 'out_for_payment' WHERE address = ? AND status = 'pending';

-- name: SetPayslipsTransaction :exec
UPDATE payslips SET tx_hash = ?, status = 'awaiting_confirmation'
WHERE address = ? AND status = 'out_for_payment';

-- name: FinalizePayslips :exec
UPDATE payslips SET status = 'completed' WHERE tx_hash = ?;

-- name: ResetPayslipsToPending :exec
UPDATE payslips SET tx_hash = NULL, status = 'pending' WHERE tx_hash = ?;

-- name: ResetPayslipsOutForPayment :exec
UPDATE payslips SET status = 'pending' WHERE address = ? AND status = 'out_for_payment';

-- name: InsertTransaction :exec
INSERT INTO transactions (hash, address, amount, status) VALUES (?, ?, ?, ?);

-- name: GetPendingTransactions :many
SELECT hash, address, amount FROM transactions WHERE status = 'awaiting_confirmation';

-- name: SetTransactionStatus :exec
UPDATE transactions SET status = ? WHERE hash = ?;

-- name: InsertValidatorAction :exec
INSERT INTO validator_actions (action, tx_hash, outcome) VALUES (?, ?, ?);

-- name: HasPendingValidatorAction :one
SELECT EXISTS(SELECT 1 FROM validator_actions WHERE action = ? AND outcome = 'pending');
