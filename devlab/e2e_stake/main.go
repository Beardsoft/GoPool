// Command e2e_stake exercises the full staker-onboarding flow against a live
// devnet: it funds a fresh wallet, signs in to the API, quotes a CreateStaker
// transaction, signs it (standing in for Nimiq Hub — Go cannot parse unsigned
// bytes, so it rebuilds the identical transaction from the quote and signs),
// submits it, and verifies the staker exists on-chain delegated to the pool.
package main

import (
	"bytes"
	"context"
	cryptoRand "crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"time"

	nimiq "github.com/NimMiniApps/nimiq-go"
	"github.com/NimMiniApps/nimiq-go/rpc"
	"github.com/NimMiniApps/nimiq-go/signer"
)

const faucetPrivHex = "3336f25f5b4272a280c8eb8c1288b39bd064dfb32ebc799459f707a0e88c4e5f"

func main() {
	ctx := context.Background()

	rpcURL := envOr("RPC_URL", "http://localhost:8647")
	apiURL := envOr("API_URL", "http://localhost:52413")
	network, err := networkFromString(envOr("NETWORK", "dev-albatross"))
	must(err, "network")
	fundLuna, err := nimiq.ParseNIM(envOr("FUND_NIM", "1000"))
	must(err, "parse fund")
	stakeLuna, err := nimiq.ParseNIM(envOr("STAKE_NIM", "500"))
	must(err, "parse stake")

	client, err := rpc.New(rpcURL, rpc.WithNetwork(network))
	must(err, "rpc new")

	var head uint32
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		if head, err = client.BlockNumber(ctx); err == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}
	must(err, "node ready")
	fmt.Printf("chain head: %d\n", head)

	policy, err := client.GetPolicy(ctx)
	must(err, "policy")
	fmt.Printf("network min stake: %s\n", policy.MinimumStake.String())

	faucetSigner, err := signer.ParsePrivateKeyHex(faucetPrivHex)
	must(err, "faucet key")
	faucetPub, err := faucetSigner.PublicKey(ctx)
	must(err, "faucet pub")
	faucetAddr, err := nimiq.AddressFromPublicKey(faucetPub)
	must(err, "faucet addr")

	keyBytes := make([]byte, 32)
	_, err = cryptoRand.Read(keyBytes)
	must(err, "rand")
	walletSigner, err := signer.ParsePrivateKeyHex(hex.EncodeToString(keyBytes))
	must(err, "wallet key")
	walletPub, err := walletSigner.PublicKey(ctx)
	must(err, "wallet pub")
	walletAddr, err := nimiq.AddressFromPublicKey(walletPub)
	must(err, "wallet addr")
	fmt.Printf("fresh wallet: %s\n", walletAddr)

	fmt.Printf("funding %s NIM from faucet %s ...\n", envOr("FUND_NIM", "1000"), faucetAddr)
	fundHash, err := client.Send(ctx, faucetSigner, walletAddr, fundLuna)
	must(err, "fund send")
	must(waitForTx(ctx, client, fundHash, 90*time.Second), "fund confirm")
	fmt.Println("funded")

	jar, _ := cookiejar.New(nil)
	api := &http.Client{Jar: jar, Timeout: 60 * time.Second}

	var chal struct {
		Nonce     string `json:"nonce"`
		Challenge string `json:"challenge"`
	}
	must(postJSON(api, apiURL+"/api/auth/challenge", map[string]any{}, &chal), "challenge")
	signed, err := nimiq.SignMessage(ctx, walletSigner, []byte(chal.Challenge))
	must(err, "sign challenge")
	var verifyResp struct {
		Address string `json:"address"`
	}
	must(postJSON(api, apiURL+"/api/auth/verify", map[string]any{
		"nonce":      chal.Nonce,
		"address":    walletAddr.String(),
		"public_key": base64.StdEncoding.EncodeToString(signed.PublicKey),
		"signature":  base64.StdEncoding.EncodeToString(signed.Signature),
	}, &verifyResp), "verify")
	fmt.Printf("signed in as: %s\n", verifyResp.Address)

	var quote struct {
		Tx                  string `json:"tx"`
		AmountLuna          int64  `json:"amount_luna"`
		FeeLuna             int64  `json:"fee_luna"`
		MinStakeLuna        int64  `json:"min_stake_luna"`
		BalanceLuna         int64  `json:"balance_luna"`
		Sender              string `json:"sender"`
		Delegate            string `json:"delegate"`
		ValidityStartHeight uint32 `json:"validity_start_height"`
	}
	must(postJSON(api, apiURL+"/api/stake/quote", map[string]any{"amount_luna": int64(stakeLuna)}, &quote), "quote")
	fmt.Printf("quote: amount=%d fee=%d min=%d balance=%d height=%d\n",
		quote.AmountLuna, quote.FeeLuna, quote.MinStakeLuna, quote.BalanceLuna, quote.ValidityStartHeight)

	raw, err := base64.StdEncoding.DecodeString(quote.Tx)
	must(err, "decode quote tx")
	if len(raw) != 188 {
		log.Fatalf("quote tx length = %d, want 188 (unsigned extended staking)", len(raw))
	}
	fmt.Printf("quote tx: %d unsigned bytes\n", len(raw))

	sender, err := nimiq.ParseAddress(quote.Sender)
	must(err, "parse sender")
	delegate, err := nimiq.ParseAddress(quote.Delegate)
	must(err, "parse delegate")
	tx, err := nimiq.NewCreateStakerTransaction(sender, &delegate, nimiq.Luna(quote.AmountLuna), nimiq.Luna(quote.FeeLuna), quote.ValidityStartHeight, network)
	must(err, "rebuild tx")
	must(nimiq.SignStakingTransaction(ctx, tx, walletSigner, walletSigner), "sign staking")
	signedBytes, err := tx.Serialize()
	must(err, "serialize signed")

	var submitResp struct {
		TXHash string `json:"tx_hash"`
	}
	must(postJSON(api, apiURL+"/api/stake/submit", map[string]any{
		"signed_tx":             hex.EncodeToString(signedBytes),
		"amount_luna":           quote.AmountLuna,
		"fee_luna":              quote.FeeLuna,
		"validity_start_height": quote.ValidityStartHeight,
	}, &submitResp), "submit")
	fmt.Printf("submitted, tx hash: %s\n", submitResp.TXHash)

	must(waitForTx(ctx, client, submitResp.TXHash, 120*time.Second), "stake confirm")
	fmt.Println("stake tx confirmed")

	staker, err := client.GetStaker(ctx, walletAddr)
	must(err, "get staker")
	fmt.Printf("on-chain staker: address=%s delegation=%s balance=%s\n",
		staker.Address, staker.Delegation, staker.Balance.String())
	if staker.Delegation != delegate.String() {
		log.Fatalf("delegation mismatch: got %s want %s", staker.Delegation, delegate)
	}
	if staker.Balance != stakeLuna {
		log.Fatalf("stake balance mismatch: got %s want %s", staker.Balance.String(), stakeLuna.String())
	}
	fmt.Println("E2E PASS: staker created on-chain and delegated to the pool")
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func must(err error, what string) {
	if err != nil {
		log.Fatalf("%s: %v", what, err)
	}
}

func networkFromString(s string) (nimiq.NetworkID, error) {
	switch s {
	case "main-albatross":
		return nimiq.NetworkMainAlbatross, nil
	case "test-albatross":
		return nimiq.NetworkTestAlbatross, nil
	case "dev-albatross":
		return nimiq.NetworkDevAlbatross, nil
	case "unit-albatross":
		return nimiq.NetworkUnitAlbatross, nil
	}
	return 0, fmt.Errorf("unknown network %q", s)
}

func postJSON(client *http.Client, url string, body any, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := client.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

func waitForTx(ctx context.Context, client *rpc.Client, hash string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		tx, err := client.GetTransaction(ctx, hash)
		if err == nil && tx != nil && tx.Succeeded() {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout waiting for tx %s", hash)
}
