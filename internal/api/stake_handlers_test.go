package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	nimiq "github.com/NimMiniApps/nimiq-go"
	"github.com/NimMiniApps/nimiq-go/rpc"
	"github.com/NimMiniApps/nimiq-go/signer"

	"github.com/Beardsoft/GoPool/internal/config"
)

// stakeRPCStub is a minimal JSON-RPC node for the staking handlers.
type stakeRPCStub struct {
	head       uint32
	minStake   nimiq.Luna
	balance    nimiq.Luna
	feePerByte float64
	srv        *httptest.Server
}

func newStakeRPCStub(t *testing.T, head uint32, minStake, balance nimiq.Luna, feePerByte float64) *stakeRPCStub {
	t.Helper()
	s := &stakeRPCStub{head: head, minStake: minStake, balance: balance, feePerByte: feePerByte}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var result any
		switch req.Method {
		case "getBlockNumber":
			result = s.head
		case "getPolicyConstants":
			result = map[string]any{
				"minimumStake": int64(s.minStake), "transactionValidityWindow": 100,
				"blocksPerBatch": 10, "batchesPerEpoch": 2, "blocksPerEpoch": 20,
			}
		case "getAccountByAddress":
			result = map[string]any{"address": "x", "balance": int64(s.balance), "type": "basic"}
		case "getMinFeePerByte":
			result = s.feePerByte
		case "sendRawTransaction":
			result = "0x" + strings.Repeat("ab", 32)
		default:
			result = nil
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	}))
	t.Cleanup(s.srv.Close)
	return s
}

// newStakeTestAPI wires an API whose RPC client points at the stub.
func newStakeTestAPI(t *testing.T, stub *stakeRPCStub) *API {
	t.Helper()
	client, err := rpc.New(stub.srv.URL, rpc.WithNetwork(nimiq.NetworkDevAlbatross))
	if err != nil {
		t.Fatal(err)
	}
	return &API{
		queries: newTestDB(t),
		cfg:     &config.Config{SessionSecret: "test-secret", Network: "dev-albatross", ValidatorAddress: testAddr},
		rpc:     client,
	}
}

func newStakeKey(t *testing.T) (nimiq.Address, *signer.PrivateKey) {
	t.Helper()
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		t.Fatal(err)
	}
	priv, err := signer.ParsePrivateKeyHex(hex.EncodeToString(keyBytes))
	if err != nil {
		t.Fatal(err)
	}
	return priv.Address(), priv
}

func stakeSessionCookie(a *API, addr nimiq.Address) *http.Cookie {
	return &http.Cookie{Name: sessionCookieName, Value: a.issueSession(addr)}
}

// signCreateStaker builds and signs a CreateStaker transaction from the quoted
// parameters and returns the signed hex bytes — standing in for Nimiq Hub in
// the browser. It rebuilds the transaction from params (the SDK refuses to
// parse unsigned bytes) rather than decoding the API's unsigned blob.
func signCreateStaker(t *testing.T, addr nimiq.Address, priv *signer.PrivateKey, amount, fee nimiq.Luna, height uint32) string {
	t.Helper()
	validator, err := nimiq.ParseAddress(testAddr)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := nimiq.NewCreateStakerTransaction(addr, &validator, amount, fee, height, nimiq.NetworkDevAlbatross)
	if err != nil {
		t.Fatal(err)
	}
	if err := nimiq.SignStakingTransaction(context.Background(), tx, priv, priv); err != nil {
		t.Fatal(err)
	}
	signed, err := tx.Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(signed)
}

// TestUnsignedStakingBytesFormat validates the hand-rolled unsigned wire format
// structurally. The SDK refuses to Serialize or ParseTransaction unsigned
// transactions, so each field is checked at its fixed Extended-format offset.
// For a CreateStaker (empty senderData, 120-byte staking payload = kind +
// optional address + 98-byte inner proof) the layout is:
//
//	0 format | 1 sender(20) | 21 senderType | 22 senderDataLen(0)
//	23 recipient(20) | 43 recipientType | 44 payloadLen | 45 payload(120)
//	165 value(8) | 173 fee(8) | 181 height(4) | 185 network | 186 flags
//	187 outer-proofLen(0)
func TestUnsignedStakingBytesFormat(t *testing.T) {
	sender, priv := newStakeKey(t)
	validator, _ := nimiq.ParseAddress(testAddr)

	tx, err := nimiq.NewCreateStakerTransaction(sender, &validator, 2000, 50, 1000, nimiq.NetworkDevAlbatross)
	if err != nil {
		t.Fatal(err)
	}
	unsigned := unsignedStakingBytes(tx)

	if len(unsigned) != 188 {
		t.Fatalf("unsigned length = %d, want 188", len(unsigned))
	}
	if unsigned[0] != byte(nimiq.FormatExtended) {
		t.Errorf("format byte = %d, want %d", unsigned[0], nimiq.FormatExtended)
	}
	if !bytes.Equal(unsigned[1:21], tx.Sender[:]) {
		t.Errorf("sender bytes mismatch: % x", unsigned[1:21])
	}
	if !bytes.Equal(unsigned[23:43], tx.Recipient[:]) {
		t.Errorf("recipient bytes mismatch: % x", unsigned[23:43])
	}
	if got := binary.BigEndian.Uint64(unsigned[165:173]); got != uint64(tx.Value) {
		t.Errorf("value = %d, want %d", got, uint64(tx.Value))
	}
	if got := binary.BigEndian.Uint64(unsigned[173:181]); got != uint64(tx.Fee) {
		t.Errorf("fee = %d, want %d", got, uint64(tx.Fee))
	}
	if got := binary.BigEndian.Uint32(unsigned[181:185]); got != tx.ValidityStartHeight {
		t.Errorf("height = %d, want %d", got, tx.ValidityStartHeight)
	}
	if unsigned[185] != byte(tx.Network) {
		t.Errorf("network = %d, want %d", unsigned[185], byte(tx.Network))
	}
	if unsigned[187] != 0x00 {
		t.Errorf("outer-proof length byte = %d, want 0 (unsigned)", unsigned[187])
	}

	// The payload prefix (kind + delegation, before the inner proof) must match
	// what the SDK builds for the same parameters.
	want, err := nimiq.NewCreateStakerTransaction(sender, &validator, 2000, 50, 1000, nimiq.NetworkDevAlbatross)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(unsigned[45:45+22], want.RecipientData[:22]) {
		t.Errorf("staking payload prefix mismatch:\n got  % x\n want % x",
			unsigned[45:45+22], want.RecipientData[:22])
	}

	// Signing must not change the stable content, only fill the proofs.
	signedTx := *tx
	if err := nimiq.SignStakingTransaction(context.Background(), &signedTx, priv, priv); err != nil {
		t.Fatal(err)
	}
	if !stakeTxMatches(&signedTx, tx) {
		t.Errorf("stakeTxMatches rejects a correctly signed copy of the quoted tx")
	}
}

func TestStakeQuote(t *testing.T) {
	stub := newStakeRPCStub(t, 1000, 1000, 1_000_000, 1.0)
	a := newStakeTestAPI(t, stub)
	addr, _ := newStakeKey(t)

	body, _ := json.Marshal(stakeQuoteRequest{AmountLuna: 2000})
	req := httptest.NewRequest(http.MethodPost, "/api/stake/quote", bytes.NewReader(body))
	req.AddCookie(stakeSessionCookie(a, addr))
	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var got stakeQuoteResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.AmountLuna != 2000 || got.MinStakeLuna != 1000 || got.BalanceLuna != 1_000_000 {
		t.Errorf("got %+v", got)
	}
	if got.FeeLuna <= 0 || got.ValidityStartHeight != 1000 {
		t.Errorf("fee/height = %d / %d", got.FeeLuna, got.ValidityStartHeight)
	}
	if got.Delegate != testAddr || got.Sender != addr.String() {
		t.Errorf("sender/delegate = %s / %s", got.Sender, got.Delegate)
	}
	if _, err := base64.StdEncoding.DecodeString(got.Tx); err != nil {
		t.Errorf("tx is not base64: %v", err)
	}
}

func TestStakeQuoteValidation(t *testing.T) {
	stub := newStakeRPCStub(t, 1000, 1000, 1_000_000, 1.0)
	a := newStakeTestAPI(t, stub)
	addr, _ := newStakeKey(t)

	post := func(amount int64) int {
		body, _ := json.Marshal(stakeQuoteRequest{AmountLuna: amount})
		req := httptest.NewRequest(http.MethodPost, "/api/stake/quote", bytes.NewReader(body))
		req.AddCookie(stakeSessionCookie(a, addr))
		rec := httptest.NewRecorder()
		a.Mux().ServeHTTP(rec, req)
		return rec.Code
	}

	if code := post(500); code != http.StatusBadRequest { // below minimum
		t.Errorf("below-min: status = %d, want 400", code)
	}
	if code := post(0); code != http.StatusBadRequest { // non-positive
		t.Errorf("zero: status = %d, want 400", code)
	}
}

func TestStakeQuoteInsufficientBalance(t *testing.T) {
	// Balance covers the stake but not stake+fee.
	stub := newStakeRPCStub(t, 1000, 1000, 1000, 100.0)
	a := newStakeTestAPI(t, stub)
	addr, _ := newStakeKey(t)

	body, _ := json.Marshal(stakeQuoteRequest{AmountLuna: 1000})
	req := httptest.NewRequest(http.MethodPost, "/api/stake/quote", bytes.NewReader(body))
	req.AddCookie(stakeSessionCookie(a, addr))
	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400, body: %s", rec.Code, rec.Body.String())
	}
}

func TestStakeQuoteRequiresSession(t *testing.T) {
	stub := newStakeRPCStub(t, 1000, 1000, 1_000_000, 1.0)
	a := newStakeTestAPI(t, stub)
	body, _ := json.Marshal(stakeQuoteRequest{AmountLuna: 2000})
	req := httptest.NewRequest(http.MethodPost, "/api/stake/quote", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestStakeSubmit(t *testing.T) {
	stub := newStakeRPCStub(t, 1000, 1000, 1_000_000, 1.0)
	a := newStakeTestAPI(t, stub)
	addr, priv := newStakeKey(t)

	// Quote first, then sign exactly what the API built.
	quoteBody, _ := json.Marshal(stakeQuoteRequest{AmountLuna: 2000})
	qreq := httptest.NewRequest(http.MethodPost, "/api/stake/quote", bytes.NewReader(quoteBody))
	qreq.AddCookie(stakeSessionCookie(a, addr))
	qrec := httptest.NewRecorder()
	a.Mux().ServeHTTP(qrec, qreq)
	if qrec.Code != http.StatusOK {
		t.Fatalf("quote status = %d, body: %s", qrec.Code, qrec.Body.String())
	}
	var quoted stakeQuoteResponse
	if err := json.NewDecoder(qrec.Body).Decode(&quoted); err != nil {
		t.Fatal(err)
	}

	signed := signCreateStaker(t, addr, priv, nimiq.Luna(quoted.AmountLuna), nimiq.Luna(quoted.FeeLuna), quoted.ValidityStartHeight)
	submitBody, _ := json.Marshal(stakeSubmitRequest{
		SignedTx:            signed,
		AmountLuna:          quoted.AmountLuna,
		FeeLuna:             quoted.FeeLuna,
		ValidityStartHeight: quoted.ValidityStartHeight,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/stake/submit", bytes.NewReader(submitBody))
	req.AddCookie(stakeSessionCookie(a, addr))
	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var got map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got["tx_hash"] == "" {
		t.Errorf("empty tx_hash: %+v", got)
	}
}

func TestStakeSubmitRejectsMismatch(t *testing.T) {
	stub := newStakeRPCStub(t, 1000, 1000, 1_000_000, 1.0)
	a := newStakeTestAPI(t, stub)
	sessionAddr, _ := newStakeKey(t)       // session staker
	otherAddr, otherPriv := newStakeKey(t) // signer of the tx

	// Build a valid tx for `otherAddr` and sign it, but submit under
	// `sessionAddr`'s cookie: the rebuilt hash (session sender) won't match.
	validator, _ := nimiq.ParseAddress(testAddr)
	tx, err := nimiq.NewCreateStakerTransaction(otherAddr, &validator, 2000, 50, 1000, nimiq.NetworkDevAlbatross)
	if err != nil {
		t.Fatal(err)
	}
	if err := nimiq.SignStakingTransaction(context.Background(), tx, otherPriv, otherPriv); err != nil {
		t.Fatal(err)
	}
	raw, err := tx.Serialize()
	if err != nil {
		t.Fatal(err)
	}

	submitBody, _ := json.Marshal(stakeSubmitRequest{
		SignedTx:            hex.EncodeToString(raw),
		AmountLuna:          2000,
		FeeLuna:             50,
		ValidityStartHeight: 1000,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/stake/submit", bytes.NewReader(submitBody))
	req.AddCookie(stakeSessionCookie(a, sessionAddr))
	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400, body: %s", rec.Code, rec.Body.String())
	}
}
