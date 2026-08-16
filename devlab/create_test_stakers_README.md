# create_test_stakers

Go script to generate test wallets, fund them from the devnet faucet, and create a staker delegating to the GoPool validator.

## Build

```bash
go build -o create_test_stakers devlab/create_test_stakers.go
```

## Run

From the project root, with devnet running:

```bash
RPC_URL=http://127.0.0.1:8647 \
NETWORK=dev-albatross \
FAUCET_PRIV_KEY=3336f25f5b4272a280c8eb8c1288b39bd064dfb32ebc799459f707a0e88c4e5f \
VALIDATOR_ADDR="NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E" \
COUNT=10 FUND_NIM=1000 STAKE_NIM=500 \
./create_test_stakers
```

Environment variables:

- `RPC_URL` – Nimiq RPC endpoint, default `http://seed1:8648`
- `NETWORK` – `dev-albatross` / `test-albatross` / `main-albatross`
- `FAUCET_PRIV_KEY` – hex private key of a pre-funded basic account in the devnet genesis
- `VALIDATOR_ADDR` – pool validator address to delegate to
- `COUNT` – number of wallets to create, default 5
- `FUND_NIM` – NIM to send to each new wallet for fees, default 1000
- `STAKE_NIM` – NIM to stake via CreateStaker, default 500 (must be >= policy.MinimumStake)

The script prints each wallet's address, private key, fund tx hash, and CreateStaker tx hash.

## Notes

- Devnet genesis includes faucet accounts:
  - `NQ87 HKRC JYGR PJN5 KQYQ 5TM1 26XX 7TNG YT27` priv `3336f25f5b4272a280c8eb8c1288b39bd064dfb32ebc799459f707a0e88c4e5f`
- Pool validator for devnet is `NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E`
- The script uses `nimiq-go` to build and sign staking transactions, so no external CLI is required.
