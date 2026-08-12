-- schema.sql
CREATE TABLE IF NOT EXISTS cursor (
    name TEXT PRIMARY KEY,
    height INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS policy_constants (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    staking_contract_address TEXT NOT NULL,
    transaction_validity_window INTEGER NOT NULL,
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

CREATE TABLE IF NOT EXISTS epochs (
    number INTEGER PRIMARY KEY,
    num_stakers INTEGER NOT NULL,
    balance INTEGER NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS stakers (
    epoch_number INTEGER NOT NULL,
    address TEXT NOT NULL,
    stake INTEGER NOT NULL,
    percentage REAL NOT NULL,
    PRIMARY KEY (epoch_number, address),
    FOREIGN KEY (epoch_number) REFERENCES epochs(number)
);

CREATE TABLE IF NOT EXISTS rewards (
    batch_number INTEGER PRIMARY KEY,
    epoch_number INTEGER NOT NULL,
    amount INTEGER NOT NULL,
    pool_fee INTEGER NOT NULL,
    num_stakers INTEGER NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (epoch_number) REFERENCES epochs(number)
);

CREATE TABLE IF NOT EXISTS payslips (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    batch_number INTEGER NOT NULL,
    address TEXT NOT NULL,
    amount INTEGER NOT NULL,
    status TEXT NOT NULL,
    tx_hash TEXT,
    FOREIGN KEY (batch_number) REFERENCES rewards(batch_number)
);

CREATE TABLE IF NOT EXISTS transactions (
    hash TEXT PRIMARY KEY,
    address TEXT NOT NULL,
    amount INTEGER NOT NULL,
    status TEXT NOT NULL,
    submitted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS validator_actions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    action TEXT NOT NULL,
    attempted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    tx_hash TEXT,
    outcome TEXT NOT NULL
);
