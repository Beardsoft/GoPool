-- schema.sql

CREATE TABLE epochs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    epoch_number INTEGER NOT NULL,
    validator_balance INTEGER NOT NULL,
    paid_out BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE payouts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    epoch_id INTEGER NOT NULL,
    staker_address TEXT NOT NULL,
    payout_tx_hash TEXT NOT NULL,
    payout_completed BOOLEAN NOT NULL DEFAULT FALSE,
    FOREIGN KEY (epoch_id) REFERENCES epochs (id)
);

CREATE TABLE pool_payouts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    epoch_id INTEGER NOT NULL,
    amount REAL NOT NULL,
    fee_percentage REAL NOT NULL,
    fee_amount REAL NOT NULL,
    fee_tx_hash TEXT NOT NULL,
    FOREIGN KEY (epoch_id) REFERENCES epochs (id)
);

CREATE TABLE stakers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    epoch_id INTEGER NOT NULL,
    address TEXT NOT NULL,
    stake INTEGER NOT NULL,
    FOREIGN KEY (epoch_id) REFERENCES epochs (id)
);
