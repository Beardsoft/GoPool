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
