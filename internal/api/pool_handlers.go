package api

import (
	"net/http"

	"github.com/Beardsoft/GoPool/internal/db"
)

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
	Network           string  `json:"network"`
}

type rewardPointResponse struct {
	EpochNumber    int64 `json:"epoch_number"`
	TotalAmount    int64 `json:"total_amount"`
	TotalFee       int64 `json:"total_fee"`
	Batches        int64 `json:"batches"`
	NumStakers     int64 `json:"num_stakers"`
	TotalStakeLuna int64 `json:"total_stake_luna"`
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
	poolDescription, contactURL, disclosure, network := "", "", "", ""
	if a.cfg != nil {
		feePct = a.cfg.PoolFeePercentage
		if a.cfg.PoolName != "" {
			poolName = a.cfg.PoolName
		}
		poolDescription, contactURL, disclosure = a.cfg.PoolDescription, a.cfg.ContactURL, a.cfg.Disclosure
		network = a.cfg.Network
	}
	writeJSON(w, http.StatusOK, poolResponse{
		CurrentEpoch:      epoch.Number,
		EpochStatus:       epoch.Status,
		NumStakers:        epoch.NumStakers,
		TotalStakeLuna:    epoch.Balance,
		TotalRewardsLuna:  totalRewards,
		PoolFeePercentage: feePct,
		PoolName:          poolName, PoolDescription: poolDescription, ContactURL: contactURL, Disclosure: disclosure,
		Network: network,
	})
}

func (a *API) handlePoolRewards(w http.ResponseWriter, r *http.Request) {
	rows, err := a.queries.ListRewardsByEpochRange(r.Context(), db.ListRewardsByEpochRangeParams{
		FromEpochNumber: 0,
		ToEpochNumber:   int64(^uint64(0) >> 1),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading pool rewards")
		return
	}

	points := make([]rewardPointResponse, len(rows))
	for i, row := range rows {
		points[i] = rewardPointResponse{
			EpochNumber:    row.EpochNumber,
			TotalAmount:    int64(row.TotalAmount.Float64),
			TotalFee:       int64(row.TotalFee.Float64),
			Batches:        row.Batches,
			NumStakers:     row.NumStakers,
			TotalStakeLuna: row.TotalStakeLuna,
		}
	}
	writeJSON(w, http.StatusOK, points)
}
