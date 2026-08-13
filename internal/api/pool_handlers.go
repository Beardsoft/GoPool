package api

import "net/http"

type poolResponse struct {
	CurrentEpoch      int64   `json:"current_epoch"`
	EpochStatus       string  `json:"epoch_status"`
	NumStakers        int64   `json:"num_stakers"`
	TotalStakeLuna    int64   `json:"total_stake_luna"`
	TotalRewardsLuna  int64   `json:"total_rewards_luna"`
	PoolFeePercentage float64 `json:"pool_fee_percentage"`
}

func (a *API) handlePool(w http.ResponseWriter, r *http.Request) {
	epoch, err := a.queries.GetLatestEpoch(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading pool state")
		return
	}
	totalRewards, err := a.queries.SumRewardsAmount(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading pool state")
		return
	}
	feePct := 0.0
	if a.cfg != nil {
		feePct = a.cfg.PoolFeePercentage
	}
	writeJSON(w, http.StatusOK, poolResponse{
		CurrentEpoch:      epoch.Number,
		EpochStatus:       epoch.Status,
		NumStakers:        epoch.NumStakers,
		TotalStakeLuna:    epoch.Balance,
		TotalRewardsLuna:  totalRewards,
		PoolFeePercentage: feePct,
	})
}
