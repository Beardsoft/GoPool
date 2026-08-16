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
    submitted_height INTEGER NOT NULL DEFAULT 0,
    submitted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_transactions_one_awaiting_per_address
ON transactions(address)
WHERE status = 'awaiting_confirmation';

CREATE TABLE IF NOT EXISTS staker_preferences (
    address TEXT PRIMARY KEY,
    compound INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS validator_actions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    action TEXT NOT NULL,
    attempted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    tx_hash TEXT,
    outcome TEXT NOT NULL,
    requested_by TEXT,
    requested_at TIMESTAMP,
    updated_at TIMESTAMP,
    state TEXT,
    error_summary TEXT,
    correlation_id TEXT
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    action_type TEXT NOT NULL,
    address TEXT NOT NULL,
    amount INTEGER NOT NULL,
    fee INTEGER NOT NULL,
    kind TEXT NOT NULL,
    status TEXT NOT NULL,
    tx_hash TEXT,
    intent_data TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    approved_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS runtime_status (
    id INTEGER PRIMARY KEY,
    heartbeat_at TIMESTAMP NOT NULL,
    daemon_version TEXT NOT NULL,
    config_hash TEXT NOT NULL,
    derived_validator_address TEXT NOT NULL,
    validator_state TEXT NOT NULL,
    last_processed_height INTEGER NOT NULL,
    chain_head INTEGER NOT NULL,
    last_tick_ms INTEGER NOT NULL,
    rpc_ok INTEGER NOT NULL,
    readiness_error TEXT
);

CREATE TABLE IF NOT EXISTS health_snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    recorded_at TIMESTAMP NOT NULL,
    chain_head INTEGER NOT NULL,
    processed_height INTEGER NOT NULL,
    tick_ms INTEGER NOT NULL,
    validator_state TEXT NOT NULL,
    live_stake INTEGER NOT NULL,
    staker_count INTEGER NOT NULL,
    pending_payout_count INTEGER NOT NULL,
    pending_payout_luna INTEGER NOT NULL,
    stuck_payout_count INTEGER NOT NULL,
    stuck_payout_luna INTEGER NOT NULL,
    wallet_balance INTEGER NOT NULL,
    rpc_ok INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS operator_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    severity TEXT NOT NULL,
    category TEXT NOT NULL,
    source TEXT NOT NULL,
    event_type TEXT NOT NULL,
    summary TEXT NOT NULL,
    context_json TEXT,
    actor_address TEXT,
    correlation_id TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS alert_deliveries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel TEXT NOT NULL,
    alert_type TEXT NOT NULL,
    destination TEXT NOT NULL,
    state TEXT NOT NULL,
    response_summary TEXT,
    attempted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    correlation_id TEXT
);

CREATE TABLE IF NOT EXISTS config_revisions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_address TEXT,
    before_json TEXT,
    after_json TEXT,
    validation_state TEXT,
    write_state TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    activated_at TIMESTAMP,
    config_hash TEXT
);

CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TIMESTAMP NOT NULL
);
