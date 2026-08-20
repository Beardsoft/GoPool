export interface PoolStatus {
  current_epoch: number;
  epoch_status: string;
  num_stakers: number;
  total_stake_luna: number;
  total_rewards_luna: number;
  pool_fee_percentage: number;
  pool_name?: string;
  pool_description?: string;
  contact_url?: string;
  telegram_url?: string;
  discord_url?: string;
  x_url?: string;
  disclosure?: string;
  network?: string;
}

export interface EpochSummary {
  number: number;
  num_stakers: number;
  balance_luna: number;
  status: string;
}

export interface StakerPosition {
  address: string;
  stake_luna: number;
  percentage: number;
}

export interface EpochDetail {
  number: number;
  status: string;
  num_stakers: number;
  balance_luna: number;
  stakers: StakerPosition[];
}

export interface RewardPoint {
  epoch_number: number;
  total_amount: number;
  total_fee: number;
  batches: number;
  num_stakers: number;
  total_stake_luna: number;
}

export interface RewardBatch {
  batch_number: number;
  epoch_number: number;
  amount_luna: number;
  pool_fee_luna: number;
  num_stakers: number;
}

export interface OperatorOverview {
  status: 'healthy' | 'attention';
  chain_lag: number;
  wallet_runway_days?: number | null;
  readiness: 'ok' | 'degraded' | 'error';
  payout_summary: Record<string, unknown>;
  validator_summary: {
    address: string;
    state: string;
    last_processed_height: number;
    last_tick_ms: number;
  };
  epoch_participation: {
    epoch: number | null;
    elected: boolean | null;
    slot_count: number | null;
    slots_total: number | null;
  };
  attention: OperatorEvent[];
  events: OperatorEvent[];
}

export interface OperatorEvent {
  id: number;
  severity: 'info' | 'warning' | 'error';
  category: string;
  source?: string;
  event_type?: string;
  summary: string;
  context_json?: string;
  actor_address?: string;
  correlation_id?: string;
  created_at?: string;
}

export interface TelemetryPoint {
  ts: string;
  value: number;
}


export interface OperatorActivityResponse {
  items: OperatorEvent[];
  next_cursor: number | null;
  has_more: boolean;
}

export interface OperatorAction {
  id: number;
  action: 'deactivate' | 'retire';
  state: 'requested' | 'processing' | 'submitted' | 'confirmed' | 'failed' | 'cancelled';
  requested_at?: string;
  updated_at?: string;
  tx_hash?: string;
  error_summary?: string;
  correlation_id?: string;
}

export interface OperatorPayout {
  hash: string;
  address: string;
  amount: number;
  status: string;
  submitted_at?: string;
  submitted_height: number;
  stuck: boolean;
  epoch_from?: number | null;
  epoch_to?: number | null;
}

export interface PageResponse<T> {
  items: T[];
  next_cursor: number | string | null;
  has_more?: boolean;
}

export interface AlertChannelStatus {
  enabled: boolean;
  configured: boolean;
  destination_hint: string;
  state: 'configured' | 'missing' | 'unavailable' | 'invalid';
}

export interface AlertDelivery {
  id: number;
  channel: string;
  alert_type: string;
  destination: string;
  state: string;
  response_summary?: string;
  attempted_at: string;
  correlation_id?: string;
}

export interface EditableConfig {
  rpc_url: string;
  network: string;
  pool_fee_wallet: string;
  pool_fee_percentage: number;
  payout_mode: 'delegate' | 'transfer';
  min_payout_luna: number;
  auto_reactivate: boolean;
  api_addr: string;
  validator_address: string;
  operator_addresses: string;
  metrics_addr: string;
  alert_telegram_enabled: boolean;
  alert_telegram_destination?: string;
  alert_webhook_enabled: boolean;
  pool_name: string;
  pool_description?: string;
  contact_url?: string;
  telegram_url?: string;
  discord_url?: string;
  x_url?: string;
  disclosure?: string;
}

export interface AlertSecrets {
  alert_telegram_token?: string;
  alert_webhook_url?: string;
}

export type SetupDraft = EditableConfig & AlertSecrets

export interface SettingsResponse {
  active_hash: string;
  daemon_hash: string;
  restart_required: boolean;
  settings: EditableConfig;
  secrets: Record<string, 'configured' | 'missing' | 'pending verification'>;
}
