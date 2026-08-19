# Validator Autostake Bootstrap Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. REQUIRED: superpowers:test-driven-development for every task.

**Goal:** After setup, GoPool faucets (testnet), registers the validator on the local node, and self-stakes leftover NIM so the operator never does that by hand.

**Architecture:** A pure `nextBootstrap` decision runs inside `waitReady`. Testnet funding is an HTTP form POST. CreateValidator uses a dedicated wallet RPC client pointed only at the bundled validator node (keys from onboarding `wallet.json`). CreateStaker is signed offline with the existing pool key and broadcast on the pool RPC. The local node URL is also a `WithFallbackEndpoints` target for ordinary RPC.

**Tech Stack:** Go 1.25, `nimiq-go` v0.4.0, existing `validator_actions` rows, Vue 3 + Vitest, Compose/Swarm.

**Design:** `docs/plans/2026-08-19-validator-autostake-design.md`

---

### Task 1: Config fields for faucet, validator RPC, and wallet JSON

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Step 1: Write the failing test**

Add cases that `LoadDaemon` copies:

- `faucet_url` and `validator_rpc_url` from JSON
- `POOL_WALLET_JSON_FILE` env into `WalletJSONFile` (path only, do not read the file here)
- empty / invalid URLs are rejected when set (`faucet_url` and `validator_rpc_url` must be HTTP(S) like `rpc_url`)
- fields are **not** on `Editable` (Settings must not show or persist them as operator knobs)

**Step 2: Verify RED**

Run: `go test ./internal/config -count=1`

Expected: FAIL — fields do not exist.

**Step 3: Minimal implementation**

On `Config` (not `Editable`):

```go
FaucetURL        string `json:"faucet_url,omitempty"`
ValidatorRPCURL  string `json:"validator_rpc_url,omitempty"`
WalletJSONFile   string `json:"-"`
```

In `LoadDaemon`, if `POOL_WALLET_JSON_FILE` is set, store the path. Validate optional URLs with the same HTTP(S) parse as `rpc_url`. Empty means unset.

**Step 4: Verify GREEN**

Run: `go test ./internal/config -count=1`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "Accept faucet, validator RPC, and wallet JSON path for autostake bootstrap."
```

---

### Task 2: Load signing and voting keys from wallet.json

**Files:**
- Create: `internal/chain/walletjson.go`
- Create: `internal/chain/walletjson_test.go`

**Step 1: Write the failing test**

```go
func TestLoadWalletJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wallet.json")
	raw := `{
	  "validator_address": "NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E",
	  "address_private_key": "aa",
	  "signing_private_key": "bb",
	  "fee_private_key": "cc",
	  "voting_secret_key": "dd",
	  "voting_public_key": "ee"
	}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := LoadWalletJSON(path)
	if err != nil {
		t.Fatal(err)
	}
	if w.SigningPrivateKey != "bb" || w.VotingSecretKey != "dd" {
		t.Fatalf("%+v", w)
	}
}

func TestLoadWalletJSONMissingKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wallet.json")
	_ = os.WriteFile(path, []byte(`{"validator_address":"NQ20"}`), 0o600)
	if _, err := LoadWalletJSON(path); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadWalletJSONAbsent(t *testing.T) {
	_, err := LoadWalletJSON(filepath.Join(t.TempDir(), "missing.json"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v", err)
	}
}
```

**Step 2: Verify RED**

Run: `go test ./internal/chain -run TestLoadWalletJSON -count=1`

Expected: FAIL — `LoadWalletJSON` undefined.

**Step 3: Minimal implementation**

Struct with `SigningPrivateKey` and `VotingSecretKey` (ignore other fields except requiring both secrets non-empty). Do not log key material.

**Step 4: Verify GREEN**

Run: `go test ./internal/chain -count=1`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/chain/walletjson.go internal/chain/walletjson_test.go
git commit -m "Load onboarding wallet.json signing and voting keys for CreateValidator."
```

---

### Task 3: Pure bootstrap decision table

**Files:**
- Create: `internal/pool/bootstrap.go`
- Create: `internal/pool/bootstrap_test.go`

**Step 1: Write the failing test**

Constants (luna): deposit `10_000_000_000` (100k NIM), min stake `10_000_000` (100 NIM), faucet target `10_100_000_000` (101k NIM), fee reserve `1_000_000` (10 NIM).

```go
func TestNextBootstrap(t *testing.T) {
	base := bootstrapSnapshot{
		Network: "test-albatross", Deposit: 10_000_000_000, MinStake: 10_000_000,
		HasWalletJSON: true, LocalConsensus: true,
	}
	cases := []struct {
		name string
		mod  func(*bootstrapSnapshot)
		want bootstrapAction
	}{
		{"testnet low balance faucets", func(s *bootstrapSnapshot) { s.Balance = 0 }, bootstrapFaucet},
		{"mainnet low balance waits", func(s *bootstrapSnapshot) { s.Network = "main-albatross"; s.Balance = 0 }, bootstrapWait},
		{"testnet at 101k registers", func(s *bootstrapSnapshot) { s.Balance = 10_100_000_000 }, bootstrapRegister},
		{"funded but no wallet json waits", func(s *bootstrapSnapshot) {
			s.Balance = 10_100_000_000
			s.HasWalletJSON = false
		}, bootstrapWait},
		{"funded but no local consensus waits", func(s *bootstrapSnapshot) {
			s.Balance = 10_100_000_000
			s.LocalConsensus = false
		}, bootstrapWait},
		{"pending register waits", func(s *bootstrapSnapshot) {
			s.Balance = 10_100_000_000
			s.PendingRegister = true
		}, bootstrapWait},
		{"validator exists self-stakes leftover", func(s *bootstrapSnapshot) {
			s.ValidatorExists = true
			s.Balance = 100_000_000 // 1000 NIM leftover
		}, bootstrapSelfStake},
		{"leftover below min+reserve waits", func(s *bootstrapSnapshot) {
			s.ValidatorExists = true
			s.Balance = 1_000_000 // 10 NIM, equal to reserve
		}, bootstrapWait},
		{"already a staker is ready", func(s *bootstrapSnapshot) {
			s.ValidatorExists = true
			s.HasSelfStaker = true
			s.Balance = 100_000_000
		}, bootstrapReady},
		{"dry-run never submits", func(s *bootstrapSnapshot) {
			s.Balance = 10_100_000_000
			s.DryRun = true
		}, bootstrapWait},
		{"mainnet with deposit registers", func(s *bootstrapSnapshot) {
			s.Network = "main-albatross"
			s.Balance = 10_100_000_000
		}, bootstrapRegister},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := base
			tc.mod(&s)
			if got := nextBootstrap(s); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestSelfStakeAmount(t *testing.T) {
	got := selfStakeAmount(100_000_000, 10_000_000)
	if got != 99_000_000 { // 1000 NIM - 10 NIM reserve
		t.Fatalf("got %d", got)
	}
	if selfStakeAmount(1_000_000, 10_000_000) != 0 {
		t.Fatal("reserve must block stake")
	}
}
```

Register only when `balance >= deposit` (policy), not only at 101k. Faucet only when `network == test-albatross` and `balance < faucetTargetNIM` and faucet URL will be checked by the caller (decision can assume faucet configured via `s.FaucetEnabled bool`). Add `FaucetEnabled` to the snapshot: mainnet false, testnet true in the base fixture.

**Step 2: Verify RED**

Run: `go test ./internal/pool -run 'TestNextBootstrap|TestSelfStakeAmount' -count=1`

Expected: FAIL — symbols undefined.

**Step 3: Minimal implementation**

```go
const (
	faucetTargetLuna = nimiq.Luna(10_100_000_000)
	feeReserveLuna   = nimiq.Luna(1_000_000)
	actionCreate     = "create"
	actionSelfStake  = "self_stake"
)

type bootstrapAction int

const (
	bootstrapWait bootstrapAction = iota
	bootstrapFaucet
	bootstrapRegister
	bootstrapSelfStake
	bootstrapReady
)
```

Priority: dry-run → wait; validator+self-staker → ready; validator+enough leftover+!pending self-stake → self-stake; !validator+faucet enabled+below target → faucet; !validator+wallet+consensus+balance>=deposit+!pending register → register; else wait.

`selfStakeAmount(balance, minStake) = balance - feeReserve` if that is `>= minStake`, else 0.

**Step 4: Verify GREEN**

Run: `go test ./internal/pool -run 'TestNextBootstrap|TestSelfStakeAmount' -count=1`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/pool/bootstrap.go internal/pool/bootstrap_test.go
git commit -m "Decide faucet, register, and self-stake from a pure bootstrap snapshot."
```

---

### Task 4: Testnet faucet client

**Files:**
- Create: `internal/pool/faucet.go`
- Create: `internal/pool/faucet_test.go`

**Step 1: Write the failing test**

Mirror `nimiq-validator-activator-go/cmd/faucet_test.go`:

- POST `application/x-www-form-urlencoded` body `address=<NQ…>`
- 200 → true
- non-200 → false
- timeout → false (httptest that sleeps past client timeout)

Do not hit the real faucet.

**Step 2: Verify RED**

Run: `go test ./internal/pool -run TestFundAddress -count=1`

Expected: FAIL.

**Step 3: Minimal implementation**

```go
func fundAddress(ctx context.Context, client *http.Client, faucetURL, address string) bool
```

`PostForm` with `url.Values{"address": {address}}`. Use a 10s default client in production. Log failures; return false.

**Step 4: Verify GREEN**

Run: `go test ./internal/pool -run TestFundAddress -count=1`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/pool/faucet.go internal/pool/faucet_test.go
git commit -m "Request testnet faucet funds with the same form POST as the activator."
```

---

### Task 5: Validator-node wallet RPC client

**Files:**
- Create: `internal/chain/walletrpc.go`
- Create: `internal/chain/walletrpc_test.go`

**Step 1: Write the failing test**

Fake JSON-RPC server (same style as `internal/pool/readiness_retry_test.go`):

- `isConsensusEstablished` → true/false
- `importRawKey` called with the address private key
- `unlockAccount` called with the address
- `sendNewValidatorTransaction` called with sender=validator=reward address, signing key, voting key, fee 500, validity `"+0"`
- result hash returned
- a client pointed at this server never calls `sendNewValidatorTransaction` on a second “pool” server (construct two servers; wallet client uses only the first)

**Step 2: Verify RED**

Run: `go test ./internal/chain -run TestWalletRPC -count=1`

Expected: FAIL.

**Step 3: Minimal implementation**

Small client wrapping `rpc.Client.Call` (or a dedicated http JSON-RPC POST) **constructed with only `validator_rpc_url`** — no fallback list.

```go
type WalletRPC struct { rpc *rpc.Client }

func (w *WalletRPC) ConsensusEstablished(ctx context.Context) (bool, error)
func (w *WalletRPC) CreateValidator(ctx context.Context, address, addressKey, signingKey, votingKey string) (string, error)
```

`CreateValidator` does importRawKey, unlockAccount(duration 0), sendNewValidatorTransaction. Match activator parameter order from `nimiq-validator-activator-go/rpc/client.go`. Parse result as a string (handle both raw string and `{"data":"..."}` if the node wraps it — assert against the fake).

**Step 4: Verify GREEN**

Run: `go test ./internal/chain -count=1`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/chain/walletrpc.go internal/chain/walletrpc_test.go
git commit -m "Call CreateValidator only on the local validator node's wallet RPC."
```

---

### Task 6: Attach local validator RPC as pool fallback

**Files:**
- Modify: `internal/chain/client.go`
- Modify: `internal/chain/client_test.go` (create if missing)
- Modify: `internal/pool/pool.go` (`Manager` fields)

**Step 1: Write the failing test**

`chain.New` with `ValidatorRPCURL` set: `client.Endpoints()` (or equivalent) is `[rpc_url, validator_rpc_url]`. `NewRPCOnly` for the API does **not** get the validator fallback (API must not depend on the validator container).

If `Endpoints()` is not exported, test via a dead primary + live fallback using `rpc.WithFallbackEndpoints` behavior: `GetBlockNumber` succeeds when primary is closed and fallback is a fake node.

Also: when `ValidatorRPCURL` is empty, only the primary is used.

**Step 2: Verify RED**

Run: `go test ./internal/chain -count=1`

Expected: FAIL — `New` ignores `ValidatorRPCURL`.

**Step 3: Minimal implementation**

```go
opts := []rpc.Option{rpc.WithNetwork(network)}
if cfg.ValidatorRPCURL != "" {
    opts = append(opts, rpc.WithFallbackEndpoints(cfg.ValidatorRPCURL))
}
client, err := rpc.New(cfg.RPCURL, opts...)
```

On `Chain`, add `Wallet *WalletRPC` when `ValidatorRPCURL != ""` (separate `rpc.New(cfg.ValidatorRPCURL)` with no fallback). `NewManager` can read `c.Wallet`.

**Step 4: Verify GREEN**

Run: `go test ./internal/chain -count=1`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/chain/client.go internal/chain/*_test.go internal/pool/pool.go
git commit -m "Use the local validator RPC as a pool fallback and as the wallet endpoint."
```

---

### Task 7: Run bootstrap inside waitReady

**Files:**
- Modify: `internal/pool/bootstrap.go`
- Modify: `internal/pool/pool.go` (`ensureReady` / `waitReady`)
- Modify: `internal/pool/readiness_retry_test.go` or new `internal/pool/bootstrap_run_test.go`

**Step 1: Write the failing test**

Extend the httptest pattern from `TestEnsureReadyRecordsHeartbeatThenSucceeds`:

1. First `getValidatorByAddress` → not found; `getAccountByAddress` (or `GetBalance`) → 0; network test-albatross; faucet httptest 200; wallet JSON on disk; wallet RPC fake with consensus + createValidator hash.
2. After faucet, next pass balance is 101k; expect `sendNewValidatorTransaction` on the **wallet** server.
3. Next pass validator exists, no staker, leftover 1000 NIM; expect a `CreateStaker` / `sendRawTransaction` on the **pool** RPC (not wallet server).
4. Final pass `ensureReady` succeeds.

Keep this as one table-driven or staged atomic counter test. Stub `GetStaker` not-found then found.

If full `ensureReady` wiring is too coupled, first test `func (m *Manager) runBootstrap(ctx) error` in isolation with a Manager whose `chain.RPC` and `chain.Wallet` are fakes.

Also: `runBootstrap` is a no-op when `cfg.DryRun`.

Record `InsertValidatorAction` with `action=create` / `self_stake` and `outcome=pending|failed`. Skip submit when `HasPendingValidatorAction`.

Readiness heartbeat `ReadinessError` text while waiting should be human, e.g. `Waiting for 100000 NIM to register the validator (have 0 NIM)`, not only the raw GetValidator error. After faucet: keep waiting with updated balance. After register submitted: `Validator registration submitted`. These strings are what setup/overview show.

**Step 2: Verify RED**

Run: `go test ./internal/pool -run 'TestRunBootstrap|TestEnsureReady' -count=1`

Expected: FAIL — bootstrap not called from `ensureReady`.

**Step 3: Minimal implementation**

In `ensureReady`, before treating GetValidator failure as terminal-for-this-pass:

1. Build `bootstrapSnapshot` from policy, balance, GetValidator/GetStaker, wallet file presence, `Wallet.ConsensusEstablished`, pending actions, `cfg.DryRun`, `cfg.FaucetURL != "" && cfg.Network == "test-albatross"`.
2. `switch nextBootstrap(s)`.
3. Faucet: `fundAddress`; 10s is already the waitReady retry (keep waitReady at 2s but **rate-limit faucet** with `lastFaucetAt` on Manager, min 10s). Register cooldown 60s (`lastRegisterAt`).
4. Register: `LoadWalletJSON`, `Wallet.CreateValidator` with `cfg.PrivateKey` as address key. Insert action row. Event `validator_registration_submitted`.
5. Self-stake: `NewCreateStakerTransaction(addr, &addr, amount, fee, head, network)`, `SignStakingTransaction` both signers = pool signer, `SendTransaction`. Event `validator_self_stake_submitted`.
6. Rewrite `recordReadinessFailure` summary from `nextBootstrap` so the UI sees the waiting sentence.

Call `runBootstrap` even when GetValidator already works (so self-stake can happen after a crash between register and stake). If snapshot is `bootstrapReady` or `bootstrapWait` after validator exists, fall through to `validatePoolWallet`.

**Step 4: Verify GREEN**

Run: `go test ./internal/pool -count=1`

Expected: PASS. Existing readiness tests still pass.

**Step 5: Commit**

```bash
git add internal/pool/bootstrap.go internal/pool/pool.go internal/pool/*bootstrap* internal/pool/readiness_retry_test.go
git commit -m "Faucet, register, and self-stake during validator readiness wait."
```

---

### Task 8: Operator copy on setup and overview

**Files:**
- Modify: `internal/api/operator_overview_handlers.go` only if readiness_error is already forwarded (it is). Prefer daemon strings; no API schema change required.
- Modify: `web/src/pages/setup/Setup.vue` / `Setup.test.ts` if the page prefixes errors; keep showing `readiness_error` verbatim.
- Modify: `web/src/pages/operator/Overview.vue` / `Overview.test.ts` only if a static “validator not found” exists — replace with readiness_error when present.
- Modify: `README.md` — remove “Stake or register the validator on chain” as a required hand step; say testnet faucets 101k and the daemon registers+self-stakes when `wallet.json` and validator RPC are mounted. Mainnet: send ≥101k NIM to the validator address.

**Step 1: Write the failing test**

Setup test: mock `/api/operator/readiness` with `readiness_error: "Waiting for 100000 NIM to register the validator (have 0 NIM)"` and expect that text on the launch screen (update `shows readiness_error text after launch` if it looks for `validator not found`).

**Step 2: Verify RED**

Run: `cd web && npx vitest run src/pages/setup/Setup.test.ts`

Expected: FAIL if copy still expects the old string.

**Step 3: Minimal implementation**

Pass through the daemon sentence. README + Swarm comments: validator RPC stays internal; set `VALIDATOR_RPC_URL` / `faucet_url` in compose.

**Step 4: Verify GREEN**

Run: `cd web && npx vitest run src/pages/setup/Setup.test.ts src/pages/operator/Overview.test.ts`

Expected: PASS.

**Step 5: Commit**

```bash
git add web/src/pages/setup web/src/pages/operator README.md
git commit -m "Show bootstrap waiting copy instead of asking operators to register by hand."
```

---

### Task 9: Compose and Swarm wiring

**Files:**
- Modify: `deployments/docker-compose.yml`
- Modify: `deployments/docker-stack.yml` and `deployments/docker-stack-test.yml`
- Modify: `deployments/SWARM.md`
- Modify: `scripts/install.sh` / `scripts/vps-onboard.sh` only if they generate compose env — set faucet URL when `--network test-albatross`

**Step 1: No unit test** — assert by reading the files in review.

Daemon service environment:

```yaml
VALIDATOR_RPC_URL: http://gopool-validator:8648
# testnet compose/stack-test only:
# faucet_url is config.json — write it into setup hints / example testnet config
POOL_WALLET_JSON_FILE: /run/secrets/gopool_wallet_json
```

Secrets: add `gopool_wallet_json` from `.secrets/wallet.json` (optional: if the file is missing, do not crash compose — document that auto-register is skipped). Swarm: `gopool_test_wallet_json` external secret from the same wallet.json used to build `node-config.toml`.

Do **not** publish `8648` on the host. Comment on `gopool-validator`: RPC is internal for CreateValidator + pool fallback only.

Put `faucet_url` in `config.testnet.json` and in `gopool_write_setup_hints` / generated config for test-albatross.

**Step 2: Verify**

`docker compose -f deployments/docker-compose.yml config` should include the new env/secret. Fix YAML until it does.

**Step 3: Commit**

```bash
git add deployments scripts config.testnet.json
git commit -m "Wire internal validator RPC, wallet.json, and testnet faucet into Compose and Swarm."
```

---

### Task 10: README and installer copy

**Files:**
- Modify: `README.md`
- Modify: `deployments/SWARM.md`
- Modify: installer success text in `scripts/install.sh` / `scripts/vps-onboard.sh` (the “You still do by hand” / “Stake or register” lines)

**Step 1:** Grep for `Stake or register` / `No RPC` and update those sentences to match the design.

**Step 2:** Commit

```bash
git add README.md deployments/SWARM.md scripts/install.sh scripts/vps-onboard.sh
git commit -m "Tell operators the daemon registers and self-stakes once the deposit is funded."
```

---

## Notes for the implementer

- Never send `wallet.json` or signing/voting keys to the API process.
- Never call `importRawKey` / `sendNewValidatorTransaction` on `rpc_url` (public nimiqscan).
- Reuse `validator_actions` + `HasPendingValidatorAction`; do not add a migration.
- `CreateValidator` stays out of `nimiq-go`; do not try to sign kind 0.
- Match existing log/event style in `runAutoReactivate` (`internal/pool/lifecycle.go`).
- After tests: `go test ./...` and `cd web && npx vitest run`.
