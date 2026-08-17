package main

import (
	"context"
	cryptoRand "crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	nimiq "github.com/NimMiniApps/nimiq-go"
	"github.com/NimMiniApps/nimiq-go/rpc"
	"github.com/NimMiniApps/nimiq-go/signer"
)

const (
	defaultRPCURL        = "http://seed1:8648"
	defaultNetwork       = "dev-albatross"
	defaultFaucetPrivKey = "3336f25f5b4272a280c8eb8c1288b39bd064dfb32ebc799459f707a0e88c4e5f"
	defaultFaucetAddr    = "NQ87 HKRC JYGR PJN5 KQYQ 5TM1 26XX 7TNG YT27"
	defaultValidatorAddr = "NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E"
	defaultCount         = 5
	defaultFundNIM       = "1000"
	defaultStakeNIM      = "500"
)

func main() {
	ctx := context.Background()

	rpcURL := getenv("RPC_URL", defaultRPCURL)
	networkStr := getenv("NETWORK", defaultNetwork)
	faucetPrivHex := getenv("FAUCET_PRIV_KEY", defaultFaucetPrivKey)
	validatorAddrStr := getenv("VALIDATOR_ADDR", defaultValidatorAddr)
	validatorAddrsEnv := getenv("VALIDATOR_ADDRS", "")
	count := getenvInt("COUNT", defaultCount)
	fundNIM := getenv("FUND_NIM", defaultFundNIM)
	stakeNIM := getenv("STAKE_NIM", defaultStakeNIM)
	fundNIMMin := getenv("FUND_NIM_MIN", "")
	fundNIMMax := getenv("FUND_NIM_MAX", "")
	stakeNIMMin := getenv("STAKE_NIM_MIN", "")
	stakeNIMMax := getenv("STAKE_NIM_MAX", "")

	networkID, err := networkFromString(networkStr)
	if err != nil {
		log.Fatalf("network: %v", err)
	}

	client, err := rpc.New(rpcURL, rpc.WithNetwork(networkID))
	if err != nil {
		log.Fatalf("rpc new: %v", err)
	}

	// Guardrail: wait for node to be healthy before proceeding
	if err := waitForNodeReady(ctx, client, 5*time.Minute); err != nil {
		log.Fatalf("node not ready: %v", err)
	}

	faucetSigner, err := signer.ParsePrivateKeyHex(faucetPrivHex)
	if err != nil {
		log.Fatalf("parse faucet key: %v", err)
	}
	pub, err := faucetSigner.PublicKey(ctx)
	if err != nil {
		log.Fatalf("faucet public key: %v", err)
	}
	faucetAddr, err := nimiq.AddressFromPublicKey(pub)
	if err != nil {
		log.Fatalf("faucet address: %v", err)
	}
	fmt.Printf("Faucet %s (derived) using key %s\n", faucetAddr, faucetPrivHex)

	validatorAddr, err := nimiq.ParseAddress(validatorAddrStr)
	if err != nil {
		log.Fatalf("parse validator: %v", err)
	}

	// Parse optional validator list for random selection
	var validatorAddrs []nimiq.Address
	if validatorAddrsEnv != "" {
		for _, s := range splitAndTrim(validatorAddrsEnv, ",") {
			addr, err := nimiq.ParseAddress(s)
			if err != nil {
				log.Fatalf("parse validator addr from list: %v", err)
			}
			validatorAddrs = append(validatorAddrs, addr)
		}
	} else {
		validatorAddrs = []nimiq.Address{validatorAddr}
	}

	policy, err := client.GetPolicy(ctx)
	if err != nil {
		log.Fatalf("get policy: %v", err)
	}
	fmt.Printf("Policy minimum stake: %s NIM\n", policy.MinimumStake.String())

	fundAmt, err := nimiq.ParseNIM(fundNIM)
	if err != nil {
		log.Fatalf("parse fund amount: %v", err)
	}
	stakeAmt, err := nimiq.ParseNIM(stakeNIM)
	if err != nil {
		log.Fatalf("parse stake amount: %v", err)
	}

	// Randomization ranges: use env if set, otherwise default to ±20% around base amounts
	var fundMin, fundMax, stakeMin, stakeMax nimiq.Luna
	if fundNIMMin != "" && fundNIMMax != "" {
		fundMin, err = nimiq.ParseNIM(fundNIMMin)
		if err != nil {
			log.Fatalf("parse FUND_NIM_MIN: %v", err)
		}
		fundMax, err = nimiq.ParseNIM(fundNIMMax)
		if err != nil {
			log.Fatalf("parse FUND_NIM_MAX: %v", err)
		}
		if fundMin > fundMax {
			log.Fatalf("FUND_NIM_MIN > FUND_NIM_MAX")
		}
	} else {
		// default ±20%
		fundMin = fundAmt * 8 / 10
		fundMax = fundAmt * 12 / 10
		if fundMin < 1 {
			fundMin = 1
		}
	}
	if stakeNIMMin != "" && stakeNIMMax != "" {
		stakeMin, err = nimiq.ParseNIM(stakeNIMMin)
		if err != nil {
			log.Fatalf("parse STAKE_NIM_MIN: %v", err)
		}
		stakeMax, err = nimiq.ParseNIM(stakeNIMMax)
		if err != nil {
			log.Fatalf("parse STAKE_NIM_MAX: %v", err)
		}
		if stakeMin > stakeMax {
			log.Fatalf("STAKE_NIM_MIN > STAKE_NIM_MAX")
		}
	} else {
		// default ±20% around base, clamped to policy minimum
		stakeMin = stakeAmt * 8 / 10
		stakeMax = stakeAmt * 12 / 10
		if stakeMin < policy.MinimumStake {
			stakeMin = policy.MinimumStake
		}
	}
	if stakeMin < policy.MinimumStake {
		log.Fatalf("stake min %s is below minimum %s", stakeMin, policy.MinimumStake)
	}

	// Guardrail: ensure faucet has enough funds
	// Estimate worst case
	requiredPerStaker := fundMax + stakeMax
	required := requiredPerStaker*nimiq.Luna(count) + nimiq.Luna(1000000)
	bal, err := client.GetBalance(ctx, faucetAddr)
	if err != nil {
		log.Fatalf("cannot get faucet balance: %v", err)
	}
	if bal < required {
		log.Fatalf("faucet balance %s insufficient for %d stakers, need ~%s", bal.String(), count, required.String())
	}
	fmt.Printf("Faucet balance %s sufficient for %d stakers\n", bal.String(), count)

	for i := 0; i < count; i++ {
		// generate random wallet
		keyBytes := make([]byte, 32)
		if _, err := cryptoRand.Read(keyBytes); err != nil {
			log.Fatalf("rand: %v", err)
		}
		privHex := hex.EncodeToString(keyBytes)
		signerObj, err := signer.ParsePrivateKeyHex(privHex)
		if err != nil {
			log.Fatalf("parse generated key: %v", err)
		}
		pub, err := signerObj.PublicKey(ctx)
		if err != nil {
			log.Fatalf("public key: %v", err)
		}
		addr, err := nimiq.AddressFromPublicKey(pub)
		if err != nil {
			log.Fatalf("address: %v", err)
		}

		fmt.Printf("\n--- Wallet %d ---\n", i+1)
		fmt.Printf("Address: %s\n", addr)
		fmt.Printf("Private key: %s\n", privHex)

		// pick random amounts and validator for this staker
		fundCurr := randomLuna(fundMin, fundMax)
		stakeCurr := randomLuna(stakeMin, stakeMax)
		// ensure stake meets minimum
		if stakeCurr < policy.MinimumStake {
			stakeCurr = policy.MinimumStake
		}
		// pick random validator
		var chosenValidator nimiq.Address
		if len(validatorAddrs) == 1 {
			chosenValidator = validatorAddrs[0]
		} else {
			idx := rand.Intn(len(validatorAddrs))
			chosenValidator = validatorAddrs[idx]
		}

		// fund
		fmt.Printf("Funding %s NIM from faucet...\n", fundCurr.String())
		hash, err := sendWithRetry(ctx, client, faucetSigner, addr, fundCurr, 3)
		if err != nil {
			log.Fatalf("fund send: %v", err)
		}
		fmt.Printf("Fund tx hash: %s\n", hash)
		if err := waitForTx(ctx, client, hash, 60*time.Second); err != nil {
			log.Fatalf("fund tx not confirmed: %v", err)
		}
		fmt.Printf("Fund tx confirmed\n")

		// create staker
		delegation := chosenValidator
		// get current height for validity start
		h, err := client.BlockNumber(ctx)
		if err != nil {
			log.Fatalf("block number: %v", err)
		}
		// build create staker tx
		tx, err := nimiq.NewCreateStakerTransaction(addr, &delegation, stakeCurr, 0, h, networkID)
		if err != nil {
			log.Fatalf("new create staker: %v", err)
		}
		// estimate fee
		fee, err := client.EstimateFee(ctx, tx)
		if err != nil {
			log.Fatalf("estimate fee: %v", err)
		}
		tx.Fee = fee
		// sign staking tx
		if err := nimiq.SignStakingTransaction(ctx, tx, signerObj, signerObj); err != nil {
			log.Fatalf("sign staking: %v", err)
		}
		stakeHash, err := sendTransactionWithRetry(ctx, client, tx, 3)
		if err != nil {
			log.Fatalf("send staking: %v", err)
		}
		fmt.Printf("CreateStaker tx hash: %s\n", stakeHash)
		if err := waitForTx(ctx, client, stakeHash, 120*time.Second); err != nil {
			log.Fatalf("stake tx not confirmed: %v", err)
		}
		fmt.Printf("CreateStaker confirmed\n")
		fmt.Printf("Delegated %s NIM to validator %s\n", stakeCurr.String(), chosenValidator)
	}

	// Summary
	stakers, err := client.GetStakersByValidatorAddress(ctx, validatorAddr)
	if err != nil {
		log.Printf("failed to list stakers: %v", err)
	} else {
		fmt.Printf("\nValidator %s now has %d stakers on-chain\n", validatorAddr, len(stakers))
		for _, s := range stakers {
			fmt.Printf("  %s  balance=%s\n", s.Address, s.Balance.String())
		}
	}

	fmt.Println("\nDone.")
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return def
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

func splitAndTrim(s, sep string) []string {
	parts := []string{}
	for _, p := range strings.Split(s, sep) {
		t := strings.TrimSpace(p)
		if t != "" {
			parts = append(parts, t)
		}
	}
	return parts
}

func randomLuna(min, max nimiq.Luna) nimiq.Luna {
	if min == max {
		return min
	}
	if min > max {
		min, max = max, min
	}
	// nimiq.Luna is int64
	span := int64(max - min)
	if span <= 0 {
		return min
	}
	// Use math/rand for simplicity
	v := rand.Int63n(span + 1)
	return min + nimiq.Luna(v)
}

func waitForNodeReady(ctx context.Context, client *rpc.Client, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastHeight uint32
	for time.Now().Before(deadline) {
		h, err := client.BlockNumber(ctx)
		if err != nil {
			log.Printf("node not reachable, retrying: %v", err)
		} else {
			if lastHeight == 0 {
				lastHeight = h
				log.Printf("node reachable, block %d", h)
			} else if h > lastHeight {
				log.Printf("node advancing, block %d -> %d", lastHeight, h)
				return nil
			}
		}
		time.Sleep(3 * time.Second)
	}
	return fmt.Errorf("node did not become ready within %s", timeout)
}

func sendWithRetry(ctx context.Context, client *rpc.Client, s nimiq.Signer, to nimiq.Address, value nimiq.Luna, attempts int) (string, error) {
	var lastErr error
	for i := 1; i <= attempts; i++ {
		hash, err := client.Send(ctx, s, to, value)
		if err == nil {
			return hash, nil
		}
		lastErr = err
		log.Printf("send attempt %d/%d failed: %v", i, attempts, err)
		time.Sleep(time.Duration(i) * 2 * time.Second)
	}
	return "", fmt.Errorf("send failed after %d attempts: %w", attempts, lastErr)
}

func sendTransactionWithRetry(ctx context.Context, client *rpc.Client, tx *nimiq.Transaction, attempts int) (string, error) {
	var lastErr error
	for i := 1; i <= attempts; i++ {
		hash, err := client.SendTransaction(ctx, tx)
		if err == nil {
			return hash, nil
		}
		lastErr = err
		log.Printf("send tx attempt %d/%d failed: %v", i, attempts, err)
		time.Sleep(time.Duration(i) * 2 * time.Second)
	}
	return "", fmt.Errorf("send tx failed after %d attempts: %w", attempts, lastErr)
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
