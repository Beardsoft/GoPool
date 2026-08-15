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
  disclosure?: string;
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
}

export interface RewardBatch {
  batch_number: number;
  epoch_number: number;
  amount_luna: number;
  pool_fee_luna: number;
  num_stakers: number;
}

export interface OperatorOverview {
  status: 'ok' | 'attention' | 'degraded';
  daemon_version?: string;
  config_hash?: string;
  chain_lag: number;
  metrics: {
    total_stake_luna: number;
    total_rewards_luna: number;
    num_stakers: number;
    wallet_runway_days: number | null;
  };
  attention: OperatorEvent[];
  validator: {
    address: string;
    state: string;
    last_processed_height: number;
  };
  telemetry_points: TelemetryPoint[];
  recent_activity: OperatorEvent[];
}

export interface OperatorEvent {
  id: number;
  severity: 'info' | 'warning' | 'error';
  category: string;
  source: string;
  event_type: string;
  summary: string;
  context_json?: string;
  actor_address?: string;
  correlation_id?: string;
  created_at: string;
}

export interface TelemetryPoint {
  ts: string;
  value: number;
}

export interface OperatorActivityFilters {
  severity?: string;
  category?: string;
  from?: string;
  to?: string;
  cursor?: string;
  limit?: number;
}

export interface OperatorActivityResponse {
  items: OperatorEvent[];
  next_cursor: string | null;
  has_more: boolean;
}
