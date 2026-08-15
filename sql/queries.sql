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

-- name: GetRequestedValidatorActions :many
SELECT id, action
FROM validator_actions
WHERE state = 'requested' OR (state IS NULL AND outcome = 'requested')
ORDER BY id;

-- name: SetValidatorActionOutcome :exec
UPDATE validator_actions SET outcome = ?, tx_hash = ? WHERE id = ?;

-- name: HasOutstandingValidatorAction :one
SELECT EXISTS(
    SELECT 1
    FROM validator_actions
    WHERE action = ?
      AND (
        state IN ('requested', 'processing', 'submitted')
        OR (state IS NULL AND outcome IN ('requested', 'pending'))
      )
);

-- name: ListEpochs :many
SELECT number, num_stakers, balance, status FROM epochs ORDER BY number DESC;

-- name: GetEpochByNumber :one
SELECT number, num_stakers, balance, status FROM epochs WHERE number = ?;

-- name: GetLatestEpoch :one
SELECT number, num_stakers, balance, status FROM epochs ORDER BY number DESC LIMIT 1;

-- name: SumRewardsAmount :one
SELECT CAST(COALESCE(SUM(amount), 0) AS INTEGER) FROM rewards;

-- name: StakerExists :one
SELECT EXISTS(SELECT 1 FROM stakers WHERE address = ?);

-- name: GetStakerLatest :one
SELECT epoch_number, stake, percentage FROM stakers WHERE address = ? ORDER BY epoch_number DESC LIMIT 1;

-- name: GetPayslipsForAddress :many
SELECT batch_number, amount, status, tx_hash FROM payslips WHERE address = ? ORDER BY batch_number DESC;

-- name: GetTransactionsForAddress :many
SELECT hash, amount, status, submitted_at FROM transactions WHERE address = ? ORDER BY submitted_at DESC;

-- name: GetStuckPayslips :many
SELECT p.id, p.batch_number, p.address, p.amount, p.status, p.tx_hash, t.submitted_at
FROM payslips p
LEFT JOIN transactions t ON t.hash = p.tx_hash
WHERE p.status IN ('out_for_payment', 'awaiting_confirmation')
ORDER BY p.id;

-- name: GetPayslipStats :one
SELECT
    CAST(COALESCE(SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END), 0) AS INTEGER) AS pending_count,
    CAST(COALESCE(SUM(CASE WHEN status = 'pending' THEN amount ELSE 0 END), 0) AS INTEGER) AS pending_luna,
    CAST(COALESCE(SUM(CASE WHEN status IN ('out_for_payment', 'awaiting_confirmation') THEN 1 ELSE 0 END), 0) AS INTEGER) AS stuck_count
FROM payslips;

-- name: GetCurrentEpochSnapshot :one
SELECT num_stakers, balance FROM epochs
WHERE status = 'in_progress'
ORDER BY number DESC
LIMIT 1;

-- name: ListRewardsByEpochRange :many
SELECT epoch_number, SUM(amount) AS total_amount, SUM(pool_fee) AS total_fee, COUNT(*) AS batches
FROM rewards
WHERE epoch_number BETWEEN ? AND ?
GROUP BY epoch_number
ORDER BY epoch_number ASC;

-- name: InsertAuditLog :one
INSERT INTO audit_logs (action_type, address, amount, fee, kind, status, intent_data)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: ListPendingAuditLogs :many
SELECT id, action_type, address, amount, fee, kind, status, intent_data, created_at
FROM audit_logs
WHERE status = 'pending'
ORDER BY created_at ASC;

-- name: ListApprovedAuditLogs :many
SELECT id, action_type, address, amount, fee, kind, status, intent_data, created_at
FROM audit_logs
WHERE status = 'approved'
ORDER BY created_at ASC;

-- name: UpdateAuditLogStatus :exec
UPDATE audit_logs
SET status = ?, approved_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: UpsertRuntimeStatus :exec
INSERT INTO runtime_status (id, heartbeat_at, daemon_version, config_hash, derived_validator_address, validator_state, last_processed_height, chain_head, last_tick_ms, rpc_ok, readiness_error)
VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET heartbeat_at=excluded.heartbeat_at, daemon_version=excluded.daemon_version, config_hash=excluded.config_hash, derived_validator_address=excluded.derived_validator_address, validator_state=excluded.validator_state, last_processed_height=excluded.last_processed_height, chain_head=excluded.chain_head, last_tick_ms=excluded.last_tick_ms, rpc_ok=excluded.rpc_ok, readiness_error=excluded.readiness_error;

-- name: GetRuntimeStatus :one
SELECT id, heartbeat_at, daemon_version, config_hash, derived_validator_address, validator_state, last_processed_height, chain_head, last_tick_ms, rpc_ok, readiness_error FROM runtime_status WHERE id = 1;

-- name: InsertHealthSnapshot :one
INSERT INTO health_snapshots (recorded_at, chain_head, processed_height, tick_ms, validator_state, live_stake, staker_count, pending_payout_count, pending_payout_luna, stuck_payout_count, stuck_payout_luna, wallet_balance, rpc_ok)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id;

-- name: InsertOperatorEvent :one
INSERT INTO operator_events (severity, category, source, event_type, summary, context_json, actor_address, correlation_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id;

-- name: InsertAlertDelivery :one
INSERT INTO alert_deliveries (channel, alert_type, destination, state, response_summary, correlation_id)
VALUES (?, ?, ?, ?, ?, ?) RETURNING id;

-- name: InsertConfigRevision :one
INSERT INTO config_revisions (actor_address, before_json, after_json, validation_state, write_state, config_hash)
VALUES (?, ?, ?, ?, ?, ?) RETURNING id;

-- name: ListConfigRevisions :many
SELECT id, actor_address, before_json, after_json, validation_state, write_state, created_at, activated_at, config_hash FROM config_revisions ORDER BY id DESC;

-- name: GetValidatorAction :one
SELECT id, action, attempted_at, tx_hash, outcome, requested_by, requested_at, updated_at, state, error_summary, correlation_id FROM validator_actions WHERE id = ?;

-- name: UpdateValidatorActionState :exec
UPDATE validator_actions SET state = ?, updated_at = CURRENT_TIMESTAMP, error_summary = ? WHERE id = ?;

-- name: MarkValidatorActionProcessing :one
UPDATE validator_actions
SET state = 'processing', updated_at = CURRENT_TIMESTAMP, error_summary = NULL
WHERE id = ?
  AND (state = 'requested' OR (state IS NULL AND outcome = 'requested'))
RETURNING id;

-- name: ReleaseValidatorActionProcessing :one
UPDATE validator_actions
SET state = 'requested', updated_at = CURRENT_TIMESTAMP, error_summary = NULL
WHERE id = ? AND state = 'processing'
RETURNING id;

-- name: SubmitValidatorAction :one
UPDATE validator_actions
SET state = 'submitted', outcome = 'pending', tx_hash = ?, updated_at = CURRENT_TIMESTAMP, error_summary = NULL
WHERE id = ? AND state = 'processing'
RETURNING id;

-- name: CompleteSubmittedValidatorAction :one
UPDATE validator_actions
SET state = ?, updated_at = CURRENT_TIMESTAMP, error_summary = ?
WHERE id = ?
  AND (state = 'submitted' OR (state IS NULL AND outcome = 'pending'))
RETURNING id;

-- name: ListOperatorEvents :many
SELECT id, severity, category, source, event_type, summary, context_json, actor_address, correlation_id, created_at FROM operator_events ORDER BY id DESC LIMIT ? OFFSET ?;

-- name: CancelRequestedValidatorAction :one
UPDATE validator_actions
SET state = 'cancelled', outcome = 'cancelled', updated_at = CURRENT_TIMESTAMP
WHERE id = ?
  AND (state = 'requested' OR (state IS NULL AND outcome = 'requested'))
RETURNING id;

-- name: ListValidatorActions :many
SELECT
    id,
    action,
    COALESCE(state, CASE outcome
        WHEN 'requested' THEN 'requested'
        WHEN 'pending' THEN 'submitted'
        WHEN 'failed' THEN 'failed'
        ELSE 'failed'
    END) AS state,
    requested_at,
    updated_at,
    tx_hash,
    error_summary,
    correlation_id
FROM validator_actions
WHERE (
    sqlc.narg(status) IS NULL
    OR COALESCE(state, CASE outcome
        WHEN 'requested' THEN 'requested'
        WHEN 'pending' THEN 'submitted'
        WHEN 'failed' THEN 'failed'
        ELSE 'failed'
    END) = sqlc.narg(status)
)
ORDER BY id DESC
LIMIT sqlc.arg(limit) OFFSET sqlc.arg(offset);

-- name: CountValidatorActions :one
SELECT COUNT(*)
FROM validator_actions
WHERE (
    sqlc.narg(status) IS NULL
    OR COALESCE(state, CASE outcome
        WHEN 'requested' THEN 'requested'
        WHEN 'pending' THEN 'submitted'
        WHEN 'failed' THEN 'failed'
        ELSE 'failed'
    END) = sqlc.narg(status)
);

-- name: InsertValidatorActionWithState :one
INSERT INTO validator_actions (action, outcome, state, requested_by, requested_at, updated_at, correlation_id)
VALUES (?, 'requested', ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?)
RETURNING id;

-- name: InsertRequestedValidatorAction :one
INSERT INTO validator_actions (action, outcome, state, requested_by, requested_at, updated_at, correlation_id)
SELECT sqlc.arg(action), 'requested', 'requested', sqlc.arg(requested_by), CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, sqlc.arg(correlation_id)
WHERE NOT EXISTS (
    SELECT 1
    FROM validator_actions
    WHERE action = sqlc.arg(action)
      AND (
        state IN ('requested', 'processing', 'submitted')
        OR (state IS NULL AND outcome IN ('requested', 'pending'))
      )
)
RETURNING id;

-- name: ListPayoutTransactions :many
SELECT hash, address, amount, status, submitted_at FROM transactions WHERE (? IS NULL OR status = ?) ORDER BY submitted_at DESC LIMIT ? OFFSET ?;

-- name: CountPayoutTransactions :one
SELECT COUNT(*) FROM transactions WHERE (? IS NULL OR status = ?);

-- name: RetryFailedPayoutPayslips :many
UPDATE payslips
SET status = 'pending', tx_hash = NULL
WHERE tx_hash = sqlc.arg(tx_hash)
  AND status = 'failed'
  AND EXISTS (
      SELECT 1
      FROM transactions AS failed_tx
      WHERE failed_tx.hash = sqlc.arg(tx_hash)
        AND failed_tx.status = 'failed'
  )
  AND NOT EXISTS (
      SELECT 1
      FROM transactions AS active_tx
      WHERE active_tx.address = payslips.address
        AND active_tx.hash != payslips.tx_hash
        AND active_tx.status = 'awaiting_confirmation'
  )
RETURNING id;

-- name: UpdatePayslipStatusFailed :exec
UPDATE payslips SET status='failed' WHERE tx_hash=?;
