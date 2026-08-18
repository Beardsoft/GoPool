# In-Process Config Activation Design

## Goal

First-run setup and later Settings saves become active without a user-run
restart command, on both Docker Compose and Swarm.

## Why not SQLite-as-source-of-truth

The API already writes `CONFIG_FILE` (`config.json`) on a volume shared with
the daemon (Compose: `data/config`; Swarm: `gopool_config`). SQLite already
stores `config_revisions` and daemon `runtime_status` heartbeats. The gap is
process lifecycle: the daemon fatals if the file is missing, `Manager.Run`
returns on readiness errors (crash loop under `on-failure`), and the API builds
its mux once so it stays in setup-only mode until the process restarts.

## Why not container self-restart

Swarm stacks use `restart_policy: condition: on-failure`. A clean exit does
not come back. An error exit crash-loops when start still fails (for example
the pool wallet is not a validator on chain yet). A bounce also aborts in-flight
payouts. In-process wait/reload avoids all three.

## Contract

- Source of truth for live settings remains `CONFIG_FILE`. SQLite remains the
  revision log and heartbeat store.
- Validator key and session secret stay deployment secrets, not in the file.
- No SSH and no `docker compose restart` / `docker service update --force`
  after a successful write.
- Compose and Swarm use the same code path.

## Daemon

1. If `config.json` is missing, wait and retry (log at info, no fatal).
2. After a successful `LoadDaemon`, enter `Manager.Run`.
3. Readiness failures (RPC down, validator not found, pool-wallet mismatch)
   record heartbeat `readiness_error` and retry. Do not return from `Run` for
   those errors.
4. On each idle tick, if the file hash differs from the running hash and no
   payout is in flight, reload: rebuild chain client + manager from the new
   file and continue. If a payout is in flight, finish it, then reload.
5. Invalid file or unreadable key: keep the previous running config (or keep
   waiting if none) and surface the error on heartbeat; do not apply the bad
   revision.

## API

1. Serve through a replaceable handler (thin `ServeHTTP` wrapper), not a mux
   captured once in `http.Server`.
2. After `POST /api/setup/complete` writes the revision, load the file, attach
   session secret + RPC, swap from setup-only mux to the full mux. The setup
   response still returns 201 with the hash.
3. After Settings save/restore, update in-memory `cfg` the same way so
   operator checks and RPC follow the new revision without a process restart.
4. `RestartRequired` on a revision means “daemon heartbeat does not yet match
   this hash”, not “the operator must restart containers”.

## UI

- Setup success: poll operator/public readiness (or `/api/pool` once the mux
  has swapped) until heartbeat `config_hash` matches the written revision, or
  show `readiness_error` (validator missing, RPC, mismatch).
- Remove the hardcoded `docker compose … restart` snippet and equivalent Swarm
  copy.
- Settings: replace “Restart required” with “Activating…” until hashes match,
  then the error if readiness failed.

## Tests

- Daemon waits until the file appears, then starts.
- Readiness error retries; process does not exit.
- Hash change with no in-flight payout reloads; with an in-flight payout,
  reload waits.
- API leaves setup-only after complete without `os.Exit`.
- Settings save updates API `cfg` and reports pending until daemon heartbeat.
- Frontend: setup/settings pages do not mention compose/swarm restart
  commands.

## Out of scope

- Moving live config into SQLite.
- Auto-registering a validator on chain.
- Changing Swarm restart policies.
