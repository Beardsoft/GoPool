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
