# One-Wallet Payout Design

## Goal

Make GoPool safe and simple to operate with one daemon-only pool wallet key. The
same wallet controls validator lifecycle operations, receives validator rewards,
and signs reward transfers or restaking payouts.

## Wallet contract

GoPool continues to load one private key from `POOL_PRIVATE_KEY_FILE`. Its
derived address is the pool wallet. At daemon startup, GoPool fetches the live
validator and requires both of these identities to match the pool wallet:

- the configured validator address;
- the validator's on-chain reward address.

A mismatch is a readiness failure. The daemon must not collect new obligations
or sign transactions, and the error must explain that the validator reward
address needs to be set to the pool wallet address. The API never receives or
mounts the key.

## Payout flow

The existing signer builds and signs both ordinary transfers and AddStake
transactions. Before creating a payout attempt, GoPool reads the pool wallet
balance and holds a payout when the wallet cannot cover its value plus fee. A
held obligation remains pending and emits a deduplicated actionable alert; it
does not create an executed audit row or a transaction hash.

After a successful broadcast, GoPool stores the chain head used as the
transaction's submission height. Only one awaiting-confirmation transaction may
exist for a staker address at a time. Confirmation and explicit operator retry
semantics remain unchanged.

## Devnet

The four-validator devnet fixture will configure the first validator's reward
address to the same `NQ20...859E` account controlled by GoPool's existing
daemon key. The current disposable database will be moved to a timestamped
backup before rebuilding the devnet, so existing diagnostic state remains
recoverable.

Live acceptance requires observing this sequence on the rebuilt devnet:

1. a reward inherent credits the pool wallet;
2. GoPool creates a payout using a non-zero submission height;
3. the node includes and macro-finalizes the payout;
4. the corresponding payslip and transaction become completed;
5. no repeated absent hashes or insufficient-balance broadcasts appear.

## Tests

Tests will cover startup rejection when the configured validator or live reward
address differs from the signer, balance gating without state mutation,
deduplicated insufficient-funds alerts, submission-height persistence, and the
existing transfer/restake choice. The full Go tests, vet, frontend tests/build,
Compose validation, and live devnet acceptance sequence are the release gates.
