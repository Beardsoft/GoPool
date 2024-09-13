-- schema.sql
CREATE TABLE IF NOT EXISTS epochs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    epoch_number INTEGER NOT NULL,
    validator_balance INTEGER NOT NULL,
    paid_out BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE TABLE IF NOT EXISTS payouts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    epoch_id INTEGER NOT NULL,
    staker_address TEXT NOT NULL,
    payout_tx_hash TEXT NOT NULL,
    payout_completed BOOLEAN NOT NULL DEFAULT FALSE,
    FOREIGN KEY (epoch_id) REFERENCES epochs (id)
);
CREATE TABLE IF NOT EXISTS pool_payouts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    epoch_id INTEGER NOT NULL,
    amount REAL NOT NULL,
    fee_percentage REAL NOT NULL,
    fee_amount REAL NOT NULL,
    fee_tx_hash TEXT NOT NULL,
    FOREIGN KEY (epoch_id) REFERENCES epochs (id)
);
CREATE TABLE IF NOT EXISTS stakers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    epoch_id INTEGER NOT NULL,
    address TEXT NOT NULL,
    stake INTEGER NOT NULL,
    percentage REAL NOT NULL,
    FOREIGN KEY (epoch_id) REFERENCES epochs (id)
);
CREATE TABLE IF NOT EXISTS policy_constants (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    staking_contract_address TEXT NOT NULL,
    coinbase_address TEXT NOT NULL,
    transaction_validity_window INTEGER NOT NULL,
    max_size_micro_body INTEGER NOT NULL,
    version INTEGER NOT NULL,
    slots INTEGER NOT NULL,
    blocks_per_batch INTEGER NOT NULL,
    batches_per_epoch INTEGER NOT NULL,
    blocks_per_epoch INTEGER NOT NULL,
    validator_deposit INTEGER NOT NULL,
    minimum_stake INTEGER NOT NULL,
    total_supply INTEGER NOT NULL,
    block_separation_time INTEGER NOT NULL,
    jail_epochs INTEGER NOT NULL,
    genesis_block_number INTEGER NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS last_processed_checkpoint (
    id INTEGER PRIMARY KEY DEFAULT 1,
    block_number BIGINT NOT NULL
);
