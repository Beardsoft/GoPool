package api

import "net/http"

type poolResponse struct {
	CurrentEpoch      int64   `json:"current_epoch"`
	EpochStatus       string  `json:"epoch_status"`
	NumStakers        int64   `json:"num_stakers"`
	TotalStakeLuna    int64   `json:"total_stake_luna"`
	TotalRewardsLuna  int64   `json:"total_rewards_luna"`
	PoolFeePercentage float64 `json:"pool_fee_percentage"`
	PoolName          string  `json:"pool_name"`
	PoolDescription   string  `json:"pool_description"`
	ContactURL        string  `json:"contact_url"`
	Disclosure        string  `json:"disclosure"`
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
	poolName := "GoPool"
	poolDescription, contactURL, disclosure := "", "", ""
	if a.cfg != nil {
		feePct = a.cfg.PoolFeePercentage
		if a.cfg.PoolName != "" {
			poolName = a.cfg.PoolName
		}
		poolDescription, contactURL, disclosure = a.cfg.PoolDescription, a.cfg.ContactURL, a.cfg.Disclosure
	}
	writeJSON(w, http.StatusOK, poolResponse{
		CurrentEpoch:      epoch.Number,
		EpochStatus:       epoch.Status,
		NumStakers:        epoch.NumStakers,
		TotalStakeLuna:    epoch.Balance,
		TotalRewardsLuna:  totalRewards,
		PoolFeePercentage: feePct,
		PoolName:          poolName, PoolDescription: poolDescription, ContactURL: contactURL, Disclosure: disclosure,
	})
}
