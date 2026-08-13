package api

import (
	"context"
	"errors"
	"net/http"

	nimiq "github.com/NimMiniApps/nimiq-go"

	"github.com/Beardsoft/GoPool/internal/db"
)

type payslipResponse struct {
	BatchNumber int64  `json:"batch_number"`
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
	if exists == 0 {
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
		payslips[i] = payslipResponse{BatchNumber: p.BatchNumber, AmountLuna: p.Amount, Status: p.Status, TxHash: p.TxHash.String}
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
	mux.HandleFunc("GET /api/me", a.requireSession(a.handleMe))
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
		writeError(w, http.StatusNotFound, "this address has never staked with this pool")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading staker")
		return
	}
	writeJSON(w, http.StatusOK, detail)
}
