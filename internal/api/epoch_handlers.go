package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
)

type epochResponse struct {
	Number      int64  `json:"number"`
	NumStakers  int64  `json:"num_stakers"`
	BalanceLuna int64  `json:"balance_luna"`
	Status      string `json:"status"`
}

type stakerResponse struct {
	Address    string  `json:"address"`
	StakeLuna  int64   `json:"stake_luna"`
	Percentage float64 `json:"percentage"`
}

type epochDetailResponse struct {
	epochResponse
	Stakers []stakerResponse `json:"stakers"`
}

type rewardBatchResponse struct {
	BatchNumber int64 `json:"batch_number"`
	EpochNumber int64 `json:"epoch_number"`
	AmountLuna  int64 `json:"amount_luna"`
	PoolFeeLuna int64 `json:"pool_fee_luna"`
	NumStakers  int64 `json:"num_stakers"`
}

func (a *API) registerEpochRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/epochs", a.handleListEpochs)
	mux.HandleFunc("GET /api/epochs/{number}", a.handleGetEpoch)
	mux.HandleFunc("GET /api/epochs/{number}/rewards", a.handleGetEpochRewards)
}

func (a *API) handleListEpochs(w http.ResponseWriter, r *http.Request) {
	rows, err := a.queries.ListEpochs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading epochs")
		return
	}
	out := make([]epochResponse, len(rows))
	for i, row := range rows {
		out[i] = epochResponse{Number: row.Number, NumStakers: row.NumStakers, BalanceLuna: row.Balance, Status: row.Status}
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) handleGetEpoch(w http.ResponseWriter, r *http.Request) {
	number, err := strconv.ParseInt(r.PathValue("number"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid epoch number")
		return
	}

	epoch, err := a.queries.GetEpochByNumber(r.Context(), number)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "epoch not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading epoch")
		return
	}

	stakerRows, err := a.queries.GetStakersForEpoch(r.Context(), number)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading stakers")
		return
	}
	stakers := make([]stakerResponse, len(stakerRows))
	for i, s := range stakerRows {
		stakers[i] = stakerResponse{Address: s.Address, StakeLuna: s.Stake, Percentage: s.Percentage}
	}

	writeJSON(w, http.StatusOK, epochDetailResponse{
		epochResponse: epochResponse{Number: epoch.Number, NumStakers: epoch.NumStakers, BalanceLuna: epoch.Balance, Status: epoch.Status},
		Stakers:       stakers,
	})
}

func (a *API) handleGetEpochRewards(w http.ResponseWriter, r *http.Request) {
	number, err := strconv.ParseInt(r.PathValue("number"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid epoch number")
		return
	}

	rows, err := a.queries.ListRewardBatchesByEpoch(r.Context(), number)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading epoch rewards")
		return
	}
	rewards := make([]rewardBatchResponse, len(rows))
	for i, row := range rows {
		rewards[i] = rewardBatchResponse{
			BatchNumber: row.BatchNumber,
			EpochNumber: row.EpochNumber,
			AmountLuna:  row.Amount,
			PoolFeeLuna: row.PoolFee,
			NumStakers:  row.NumStakers,
		}
	}
	writeJSON(w, http.StatusOK, rewards)
}
