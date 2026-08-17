package api

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	nimiq "github.com/NimMiniApps/nimiq-go"

	"github.com/Beardsoft/GoPool/internal/db"
)

type payslipResponse struct {
	BatchNumber int64  `json:"batch_number"`
	EpochNumber int64  `json:"epoch_number"`
	AmountLuna  int64  `json:"amount_luna"`
	Status      string `json:"status"`
	TxHash      string `json:"tx_hash,omitempty"`
}

type transactionResponse struct {
	Hash       string `json:"hash"`
	AmountLuna int64  `json:"amount_luna"`
	Status     string `json:"status"`
}

type stakerDetailResponse struct {
	Address      string                `json:"address"`
	StakeLuna    int64                 `json:"stake_luna"`
	Percentage   float64               `json:"percentage"`
	Payslips     []payslipResponse     `json:"payslips"`
	Transactions []transactionResponse `json:"transactions"`
}

// meResponse extends the public staker detail with the logged-in staker's live
// profile: pending/paid totals, current delegation state, and compounding choice.
type meResponse struct {
	stakerDetailResponse
	PendingLuna int64 `json:"pending_luna"`
	PaidLuna    int64 `json:"paid_luna"`
	Delegated   bool  `json:"delegated"`
	Compound    bool  `json:"compound"`
}

type stakerHistoryEpoch struct {
	EpochNumber int64   `json:"epoch_number"`
	StakeLuna   int64   `json:"stake_luna"`
	Percentage  float64 `json:"percentage"`
	RewardLuna  int64   `json:"reward_luna"`
}

type stakerHistoryResponse struct {
	Address              string               `json:"address"`
	Epochs               []stakerHistoryEpoch `json:"epochs"`
	CumulativeRewardLuna int64                `json:"cumulative_reward_luna"`
}

var errStakerNotFound = errors.New("api: staker not found")

// stakerDetail fans out the reads GET /api/stakers/{address} and GET /api/me
// both need, taking a pre-parsed address so callers that already resolved
// one from a session (Task 7) don't reparse it.
func stakerDetail(ctx context.Context, q *db.Queries, addr nimiq.Address) (stakerDetailResponse, error) {
	lookup := addr.String()

	exists, err := q.StakerExists(ctx, lookup)
	if err != nil {
		return stakerDetailResponse{}, err
	}
	if !exists {
		return stakerDetailResponse{}, errStakerNotFound
	}

	latest, err := q.GetStakerLatest(ctx, lookup)
	if err != nil {
		return stakerDetailResponse{}, err
	}

	payslipRows, err := q.GetPayslipsForAddress(ctx, lookup)
	if err != nil {
		return stakerDetailResponse{}, err
	}
	payslips := make([]payslipResponse, len(payslipRows))
	for i, p := range payslipRows {
		payslips[i] = payslipResponse{BatchNumber: p.BatchNumber, EpochNumber: p.EpochNumber, AmountLuna: p.Amount, Status: p.Status, TxHash: p.TxHash.String}
	}

	txRows, err := q.GetTransactionsForAddress(ctx, lookup)
	if err != nil {
		return stakerDetailResponse{}, err
	}
	txs := make([]transactionResponse, len(txRows))
	for i, tx := range txRows {
		txs[i] = transactionResponse{Hash: tx.Hash, AmountLuna: tx.Amount, Status: tx.Status}
	}

	return stakerDetailResponse{
		Address: addr.String(), StakeLuna: latest.Stake, Percentage: latest.Percentage,
		Payslips: payslips, Transactions: txs,
	}, nil
}

func (a *API) registerStakerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/stakers/{address}", a.handleGetStaker)
	mux.HandleFunc("GET /api/stakers/{address}/history", a.handleGetStakerHistory)
	mux.HandleFunc("GET /api/stakers/{address}/payslips.csv", a.handleStakerPayslipsCSV)
	mux.HandleFunc("GET /api/me", a.requireSession(a.handleMe))
	mux.HandleFunc("GET /api/me/history", a.requireSession(a.handleMeHistory))
	mux.HandleFunc("GET /api/me/payslips.csv", a.requireSession(a.handleMePayslipsCSV))
	mux.HandleFunc("GET /api/me/preference", a.requireSession(a.handleGetMyPreference))
	mux.HandleFunc("PUT /api/me/preference", a.requireSession(a.handlePutMyPreference))
}

func (a *API) handleGetStaker(w http.ResponseWriter, r *http.Request) {
	addr, err := nimiq.ParseAddress(r.PathValue("address"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid address")
		return
	}

	detail, err := stakerDetail(r.Context(), a.queries, addr)
	if errors.Is(err, errStakerNotFound) {
		writeError(w, http.StatusNotFound, "no staker with this address in this pool")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading staker")
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (a *API) handleMe(w http.ResponseWriter, r *http.Request) {
	addr, ok := addressFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not logged in")
		return
	}
	detail, err := stakerDetail(r.Context(), a.queries, addr)
	if errors.Is(err, errStakerNotFound) {
		// A signed-in user who has never staked is not an error: show an
		// empty dashboard instead of bouncing them back to the login screen.
		detail = stakerDetailResponse{Address: addr.String(), Payslips: []payslipResponse{}, Transactions: []transactionResponse{}}
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "loading staker")
		return
	}

	var pending, paid int64
	for _, p := range detail.Payslips {
		switch p.Status {
		case "completed":
			paid += p.AmountLuna
		case "pending", "out_for_payment", "awaiting_confirmation":
			pending += p.AmountLuna
		}
	}

	delegated := false
	if a.rpc != nil && a.cfg != nil && a.cfg.ValidatorAddress != "" {
		if staker, rerr := a.rpc.GetStaker(r.Context(), addr); rerr == nil {
			delegated = staker.Delegation == a.cfg.ValidatorAddress
		}
	}
	compound, cerr := a.effectiveCompound(r.Context(), addr.String())
	if cerr != nil {
		writeError(w, http.StatusInternalServerError, "loading preference")
		return
	}

	writeJSON(w, http.StatusOK, meResponse{
		stakerDetailResponse: detail,
		PendingLuna:          pending,
		PaidLuna:             paid,
		Delegated:            delegated,
		Compound:             compound,
	})
}

func stakerHistory(ctx context.Context, q *db.Queries, addr nimiq.Address) (stakerHistoryResponse, error) {
	lookup := addr.String()
	exists, err := q.StakerExists(ctx, lookup)
	if err != nil {
		return stakerHistoryResponse{}, err
	}
	if !exists {
		return stakerHistoryResponse{}, errStakerNotFound
	}

	epochRows, err := q.GetStakerEpochs(ctx, lookup)
	if err != nil {
		return stakerHistoryResponse{}, err
	}
	rewardRows, err := q.GetStakerRewardsByEpoch(ctx, lookup)
	if err != nil {
		return stakerHistoryResponse{}, err
	}

	rewardsMap := make(map[int64]int64, len(rewardRows))
	var cumulative int64
	for _, r := range rewardRows {
		rewardsMap[r.EpochNumber] = r.Reward
		cumulative += r.Reward
	}

	epochs := make([]stakerHistoryEpoch, 0, len(epochRows))
	for _, e := range epochRows {
		reward := rewardsMap[e.EpochNumber]
		epochs = append(epochs, stakerHistoryEpoch{
			EpochNumber: e.EpochNumber,
			StakeLuna:   e.Stake,
			Percentage:  e.Percentage,
			RewardLuna:  reward,
		})
	}

	return stakerHistoryResponse{
		Address:              lookup,
		Epochs:               epochs,
		CumulativeRewardLuna: cumulative,
	}, nil
}

func (a *API) handleGetStakerHistory(w http.ResponseWriter, r *http.Request) {
	addr, err := nimiq.ParseAddress(r.PathValue("address"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid address")
		return
	}
	hist, err := stakerHistory(r.Context(), a.queries, addr)
	if errors.Is(err, errStakerNotFound) {
		writeError(w, http.StatusNotFound, "no staker with this address in this pool")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading staker history")
		return
	}
	writeJSON(w, http.StatusOK, hist)
}

func (a *API) handleMeHistory(w http.ResponseWriter, r *http.Request) {
	addr, ok := addressFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not logged in")
		return
	}
	hist, err := stakerHistory(r.Context(), a.queries, addr)
	if errors.Is(err, errStakerNotFound) {
		hist = stakerHistoryResponse{Address: addr.String(), Epochs: []stakerHistoryEpoch{}}
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "loading staker history")
		return
	}
	writeJSON(w, http.StatusOK, hist)
}

// writePayslipsCSV streams a staker's payslips as a downloadable CSV, joining
// each batch to its epoch so stakers can reconcile payouts against the record.
func writePayslipsCSV(w http.ResponseWriter, r *http.Request, q *db.Queries, addr string) {
	rows, err := q.GetPayslipsForAddress(r.Context(), addr)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading payslips")
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="payslips.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"epoch_number", "batch_number", "amount_luna", "amount_nim", "status", "tx_hash"})
	for _, p := range rows {
		_ = cw.Write([]string{
			strconv.FormatInt(p.EpochNumber, 10),
			strconv.FormatInt(p.BatchNumber, 10),
			strconv.FormatInt(p.Amount, 10),
			strconv.FormatFloat(float64(p.Amount)/100000, 'f', -1, 64),
			p.Status,
			p.TxHash.String,
		})
	}
	cw.Flush()
}

func (a *API) handleStakerPayslipsCSV(w http.ResponseWriter, r *http.Request) {
	addr, err := nimiq.ParseAddress(r.PathValue("address"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid address")
		return
	}
	if exists, err := a.queries.StakerExists(r.Context(), addr.String()); err != nil {
		writeError(w, http.StatusInternalServerError, "loading staker")
		return
	} else if !exists {
		writeError(w, http.StatusNotFound, "no staker with this address in this pool")
		return
	}
	writePayslipsCSV(w, r, a.queries, addr.String())
}

func (a *API) handleMePayslipsCSV(w http.ResponseWriter, r *http.Request) {
	addr, ok := addressFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not logged in")
		return
	}
	writePayslipsCSV(w, r, a.queries, addr.String())
}

// effectiveCompound returns the stored preference, or the value implied by the
// global payout_mode when the staker has not chosen yet.
func (a *API) effectiveCompound(ctx context.Context, address string) (bool, error) {
	pref, err := a.queries.GetStakerPreference(ctx, address)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return a.cfg.PayoutMode == "delegate", nil
		}
		return false, err
	}
	return pref == 1, nil
}

func (a *API) handleGetMyPreference(w http.ResponseWriter, r *http.Request) {
	addr, ok := addressFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not logged in")
		return
	}
	compound, err := a.effectiveCompound(r.Context(), addr.String())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading preference")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"compound": compound})
}

func (a *API) handlePutMyPreference(w http.ResponseWriter, r *http.Request) {
	addr, ok := addressFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not logged in")
		return
	}
	var body struct {
		Compound bool `json:"compound"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := a.queries.UpsertStakerPreference(r.Context(), db.UpsertStakerPreferenceParams{
		Address: addr.String(), Compound: boolToInt(body.Compound),
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "saving preference")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"compound": body.Compound})
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
