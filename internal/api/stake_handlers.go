package api

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

	nimiq "github.com/NimMiniApps/nimiq-go"

	"github.com/Beardsoft/GoPool/internal/chain"
	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/logger"

	"go.uber.org/zap"
)

// Staker onboarding: a signed-in staker delegates NIM to the pool. The API
// builds an unsigned CreateStaker transaction, the staker's wallet signs it in
// Nimiq Hub (the key never reaches the API), and the API verifies the signed
// bytes against what it would build for that staker before broadcasting. The
// pool's existing per-epoch staker tracking then picks the new staker up.

type stakeQuoteRequest struct {
	AmountLuna int64 `json:"amount_luna"`
}

type stakeQuoteResponse struct {
	Tx                  string `json:"tx"`
	AmountLuna          int64  `json:"amount_luna"`
	FeeLuna             int64  `json:"fee_luna"`
	MinStakeLuna        int64  `json:"min_stake_luna"`
	BalanceLuna         int64  `json:"balance_luna"`
	Sender              string `json:"sender"`
	Delegate            string `json:"delegate"`
	ValidityStartHeight uint32 `json:"validity_start_height"`
}

type stakeSubmitRequest struct {
	SignedTx            string `json:"signed_tx"`
	AmountLuna          int64  `json:"amount_luna"`
	FeeLuna             int64  `json:"fee_luna"`
	ValidityStartHeight uint32 `json:"validity_start_height"`
}

func (a *API) registerStakeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/stake/quote", a.requireSession(a.handleStakeQuote))
	mux.HandleFunc("POST /api/stake/submit", a.requireSession(a.handleStakeSubmit))
}

// stakeOnboardingReady reports whether the API can build onboarding
// transactions: it needs a live RPC client and a configured pool validator.
func (a *API) stakeOnboardingReady() (nimiq.NetworkID, nimiq.Address, error) {
	if a.rpc == nil || a.cfg == nil || a.cfg.ValidatorAddress == "" {
		return 0, nimiq.Address{}, fmt.Errorf("pool not configured for onboarding")
	}
	network, err := chain.NetworkID(a.cfg.Network)
	if err != nil {
		return 0, nimiq.Address{}, err
	}
	validator, err := nimiq.ParseAddress(a.cfg.ValidatorAddress)
	if err != nil {
		return 0, nimiq.Address{}, fmt.Errorf("invalid pool validator address: %w", err)
	}
	return network, validator, nil
}

// handleStakeQuote builds an unsigned CreateStaker transaction for the
// session staker and returns its wire bytes for the wallet to sign.
func (a *API) handleStakeQuote(w http.ResponseWriter, r *http.Request) {
	addr, ok := addressFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not logged in")
		return
	}

	var req stakeQuoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AmountLuna <= 0 {
		writeError(w, http.StatusBadRequest, "amount_luna must be a positive integer")
		return
	}

	network, validator, err := a.stakeOnboardingReady()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	ctx := r.Context()
	head, err := a.rpc.BlockNumber(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, "reading chain head")
		return
	}
	policy, err := a.rpc.GetPolicy(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, "reading chain policy")
		return
	}
	balance, err := a.rpc.GetBalance(ctx, addr)
	if err != nil {
		writeError(w, http.StatusBadGateway, "reading balance")
		return
	}

	amount := nimiq.Luna(req.AmountLuna)
	if amount < policy.MinimumStake {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("amount is below the network minimum stake of %d luna", int64(policy.MinimumStake)))
		return
	}

	probe, err := nimiq.NewCreateStakerTransaction(addr, &validator, amount, 0, head, network)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "building stake transaction")
		return
	}
	fee, err := a.rpc.EstimateFee(ctx, probe)
	if err != nil {
		writeError(w, http.StatusBadGateway, "estimating fee")
		return
	}
	total, err := amount.Add(fee)
	if err != nil || balance < total {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("insufficient balance: need %d luna (amount + fee) but have %d", int64(total), int64(balance)))
		return
	}

	tx, err := nimiq.NewCreateStakerTransaction(addr, &validator, amount, fee, head, network)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "building stake transaction")
		return
	}
	raw := unsignedStakingBytes(tx)

	writeJSON(w, http.StatusOK, stakeQuoteResponse{
		Tx:                  base64.StdEncoding.EncodeToString(raw),
		AmountLuna:          int64(amount),
		FeeLuna:             int64(fee),
		MinStakeLuna:        int64(policy.MinimumStake),
		BalanceLuna:         int64(balance),
		Sender:              addr.String(),
		Delegate:            validator.String(),
		ValidityStartHeight: head,
	})
}

// handleStakeSubmit verifies a wallet-signed CreateStaker transaction and
// broadcasts it. Verification rebuilds the unsigned transaction the API would
// build for this staker and compares its stable fields — the staking payload's
// trailing inner proof is zero when unsigned and filled when signed, so the
// two transactions hash differently and cannot be compared that way. It then
// checks the outer signature authorizes the session staker over the exact
// signed content.
func (a *API) handleStakeSubmit(w http.ResponseWriter, r *http.Request) {
	addr, ok := addressFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not logged in")
		return
	}

	var req stakeSubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SignedTx == "" {
		writeError(w, http.StatusBadRequest, "signed_tx is required")
		return
	}

	network, validator, err := a.stakeOnboardingReady()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	signed, err := hex.DecodeString(req.SignedTx)
	if err != nil {
		writeError(w, http.StatusBadRequest, "signed_tx must be hex-encoded")
		return
	}
	// ParseTransaction rejects transactions without a proof, so a parsed
	// transaction is signed.
	tx, err := nimiq.ParseTransaction(signed)
	if err != nil {
		writeError(w, http.StatusBadRequest, "signed_tx is not a valid signed transaction")
		return
	}

	expected, err := nimiq.NewCreateStakerTransaction(addr, &validator, nimiq.Luna(req.AmountLuna), nimiq.Luna(req.FeeLuna), req.ValidityStartHeight, network)
	if err != nil {
		writeError(w, http.StatusBadRequest, "quoted stake parameters are invalid")
		return
	}
	if !stakeTxMatches(tx, expected) {
		writeError(w, http.StatusBadRequest, "signed transaction does not match the quoted stake")
		return
	}
	proof, err := nimiq.ParseSignatureProof(tx.Proof)
	if err != nil {
		writeError(w, http.StatusBadRequest, "signed_tx proof is malformed: "+err.Error())
		return
	}
	if err := proof.Verify(tx); err != nil {
		writeError(w, http.StatusBadRequest, "signature verification failed: "+err.Error())
		return
	}

	hash, err := a.rpc.SendRawTransaction(r.Context(), signed)
	if err != nil {
		writeError(w, http.StatusBadGateway, "broadcasting stake transaction: "+err.Error())
		return
	}

	if _, err := a.queries.InsertOperatorEvent(r.Context(), db.InsertOperatorEventParams{
		Severity:  "info",
		Category:  "onboarding",
		Source:    "api",
		EventType: "stake_created",
		Summary:   fmt.Sprintf("New staker delegated %d luna to the pool", req.AmountLuna),
		ContextJson: sql.NullString{
			String: fmt.Sprintf(`{"amount_luna":%d,"tx_hash":"%s"}`, req.AmountLuna, hash), Valid: true,
		},
		ActorAddress: sql.NullString{String: addr.String(), Valid: true},
	}); err != nil {
		// The stake is already broadcast; a failed audit row must not undo it.
		logger.Logger.Error("recording stake_created event", zap.String("tx", hash), zap.Error(err))
	}

	writeJSON(w, http.StatusOK, map[string]string{"tx_hash": hash})
}

// stakingInnerProofLen is the size of the staker's inner signature proof
// embedded at the end of the staking payload (nimiq's emptyProofLen: 1
// algorithm byte + 32-byte key + 1-byte Merkle path + 64-byte signature).
const stakingInnerProofLen = 1 + 32 + 1 + 64

// stakeTxMatches reports whether a signed staking transaction carries exactly
// the content of the unsigned transaction the API quoted. The trailing inner
// proof of the staking payload is zero in the quoted transaction and filled in
// the signed one, so the payload is compared up to that boundary; everything
// else must match byte for byte.
func stakeTxMatches(got, want *nimiq.Transaction) bool {
	if got.Sender != want.Sender || got.SenderType != want.SenderType ||
		got.Recipient != want.Recipient || got.RecipientType != want.RecipientType ||
		got.Value != want.Value || got.Fee != want.Fee ||
		got.ValidityStartHeight != want.ValidityStartHeight ||
		got.Network != want.Network || got.Flags != want.Flags ||
		len(got.RecipientData) != len(want.RecipientData) {
		return false
	}
	prefix := len(want.RecipientData) - stakingInnerProofLen
	return bytes.Equal(got.RecipientData[:prefix], want.RecipientData[:prefix])
}

// unsignedStakingBytes serializes a staking transaction in the Extended wire
// format with an empty proof. The SDK's Serialize refuses unsigned transactions
// (a node would reject them), but the browser's Nimiq Hub needs exactly these
// bytes to sign. Staking transactions are always Extended — they carry
// recipient data — so the format is fixed. The format is validated by
// round-tripping through nimiq.ParseTransaction (see stake_handlers_test.go).
func unsignedStakingBytes(t *nimiq.Transaction) []byte {
	if t.Format() != nimiq.FormatExtended {
		panic("unsignedStakingBytes: staking transactions are always extended format")
	}
	var b []byte
	b = append(b, byte(nimiq.FormatExtended))
	b = append(b, t.Sender[:]...)
	b = append(b, byte(t.SenderType))
	b = appendUvarint(b, uint64(len(t.SenderData)))
	b = append(b, t.SenderData...)
	b = append(b, t.Recipient[:]...)
	b = append(b, byte(t.RecipientType))
	b = appendUvarint(b, uint64(len(t.RecipientData)))
	b = append(b, t.RecipientData...)
	b = binary.BigEndian.AppendUint64(b, uint64(t.Value))
	b = binary.BigEndian.AppendUint64(b, uint64(t.Fee))
	b = binary.BigEndian.AppendUint32(b, t.ValidityStartHeight)
	b = append(b, byte(t.Network))
	b = append(b, byte(t.Flags))
	b = appendUvarint(b, 0) // empty proof
	return b
}

// appendUvarint appends postcard's LEB128 unsigned varint encoding of v.
func appendUvarint(b []byte, v uint64) []byte {
	for v >= 0x80 {
		b = append(b, byte(v)|0x80)
		v >>= 7
	}
	return append(b, byte(v))
}
