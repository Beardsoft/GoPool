-- queries.sql
-- Get the ID of an epoch
-- name: GetEpochID :one
SELECT id
FROM epochs
WHERE epoch_number = ?;
-- Get the validator balance for a specific epoch ID
-- name: GetValidatorBalance :one
SELECT validator_balance
FROM epochs
WHERE id = ?;
-- Mark an epoch as paid
-- name: MarkEpochAsPaid :exec
UPDATE epochs
SET paid_out = 1
WHERE id = ?;
-- Insert a new epoch with its balance
-- name: InsertEpoch :one
INSERT INTO epochs (epoch_number, validator_balance)
VALUES (?, ?)
RETURNING id;
-- name: InsertStakerWithPercentage :exec
INSERT INTO stakers (epoch_id, address, stake, percentage)
VALUES (?, ?, ?, ?);
-- Insert a new payout
-- name: InsertPayout :exec
INSERT INTO payouts (
        epoch_id,
        staker_address,
        payout_tx_hash,
        payout_completed
    )
VALUES (?, ?, ?, 1);
-- Insert a pool payout
-- name: InsertPoolPayout :exec
INSERT INTO pool_payouts (
        epoch_id,
        amount,
        fee_percentage,
        fee_amount,
        fee_tx_hash
    )
VALUES (?, ?, ?, ?, ?);
-- Get the count of incomplete payouts for an epoch
-- name: CountIncompletePayouts :one
SELECT COUNT(*)
FROM payouts
WHERE epoch_id = ?
    AND payout_completed = 0;
-- name: UpdateEpochBalance :exec
UPDATE epochs
SET validator_balance = ?
WHERE epoch_number = ?;
-- name: InsertStaker :exec
INSERT INTO stakers (epoch_id, address, stake)
VALUES (?, ?, ?);
-- name: GetStakersForEpoch :many
SELECT address AS staker_address,
    stake
FROM stakers
WHERE epoch_id = ?;
-- name: InsertPolicyConstants :exec
INSERT INTO policy_constants (
        staking_contract_address,
        coinbase_address,
        transaction_validity_window,
        max_size_micro_body,
        version,
        slots,
        blocks_per_batch,
        batches_per_epoch,
        blocks_per_epoch,
        validator_deposit,
        minimum_stake,
        total_supply,
        block_separation_time,
        jail_epochs,
        genesis_block_number
    )
VALUES (
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?
    );
-- name: GetLatestPolicyConstants :one
SELECT staking_contract_address,
    coinbase_address,
    transaction_validity_window,
    max_size_micro_body,
    version,
    slots,
    blocks_per_batch,
    batches_per_epoch,
    blocks_per_epoch,
    validator_deposit,
    minimum_stake,
    total_supply,
    block_separation_time,
    jail_epochs,
    genesis_block_number
FROM policy_constants
ORDER BY id DESC
LIMIT 1;

-- name: GetLastProcessedCheckpoint :one
SELECT block_number FROM last_processed_checkpoint LIMIT 1;

-- name: UpdateLastProcessedCheckpoint :exec
INSERT INTO last_processed_checkpoint (block_number)
VALUES (?)
ON CONFLICT (id) DO UPDATE SET block_number = EXCLUDED.block_number;
