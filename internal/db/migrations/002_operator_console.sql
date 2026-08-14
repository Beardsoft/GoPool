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

ALTER TABLE validator_actions ADD COLUMN requested_by TEXT;
ALTER TABLE validator_actions ADD COLUMN requested_at TIMESTAMP;
ALTER TABLE validator_actions ADD COLUMN updated_at TIMESTAMP;
ALTER TABLE validator_actions ADD COLUMN state TEXT;
ALTER TABLE validator_actions ADD COLUMN error_summary TEXT;
ALTER TABLE validator_actions ADD COLUMN correlation_id TEXT;
