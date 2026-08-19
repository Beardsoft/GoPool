# GoPool

Run your own [Nimiq](https://nimiq.com) staking pool on a VPS.

People delegate NIM to your validator. You produce blocks. GoPool tracks who staked, records rewards, and pays them out. You also get a website for the pool and an operator dashboard.

You do **not** need to know Go, compile anything, or run Nimiq RPC yourself.

## What you need

- An Ubuntu or Debian VPS (2 CPU / 4 GB RAM is comfortable)
- A domain name, for example `pool.yourname.com`
- **101,000 NIM** to register the validator (real NIM on mainnet, testnet coins if you are practising)

## 1. Point the domain at the server

Pick the setup that matches how you host things.

**The VPS is on the public internet**

1. Create a DNS **A** record: `pool.yourname.com` → the VPS IP address.
2. In the cloud firewall (and `ufw` if you use it), open ports **80** and **443**.

**You already use Nginx Proxy Manager, Traefik, or Cloudflare in front**

1. Point the domain at that proxy, not at the VPS.
2. After install, forward `https://pool.yourname.com` to `http://VPS-IP:80`.

## 2. Install

SSH into the VPS, then:

```bash
curl -fsSL https://raw.githubusercontent.com/Beardsoft/GoPool/master/scripts/install.sh \
  | sudo bash -s -- --domain pool.yourname.com
```

To try testnet first (recommended once, before mainnet):

```bash
curl -fsSL https://raw.githubusercontent.com/Beardsoft/GoPool/master/scripts/install.sh \
  | sudo bash -s -- --domain pool.yourname.com --network test-albatross
```

That installs Docker if needed, creates a validator wallet, and starts the pool, the website, and the validator node. It prints a **setup link** — copy the whole thing (it includes a one-time token).

## 3. Finish setup in the browser

Open the setup link. Confirm the pre-filled wallet and network, set the pool name and fee, then launch.

Wait until the dashboard looks live. You do not need to restart Docker after this.

## 4. Back up the wallet now

On the server the keys live in:

```text
/opt/gopool/.secrets/wallet.json
```

Copy that file somewhere safe **offline** (USB stick, password manager, printed backup). If the VPS dies and you only have this file, you can recover. If you lose both, the validator is gone.

Keep `wallet.json` on the server until the validator is registered (GoPool needs those keys to register). After that you can delete the server copy **if** you still have the offline backup.

## 5. Fund the validator

Send **at least 101,000 NIM** to the validator address shown in setup.

- **Mainnet:** send real NIM.
- **Testnet:** GoPool asks the public faucet **once**. That faucet is limited to about once per 24 hours **per public IP**. If it is rate-limited, send 101,000 testnet NIM yourself.

As soon as the balance is there, GoPool registers the validator and stakes the leftover. A waiting / readiness message on the dashboard until that finishes is normal.

The validator node also downloads the chain the first time. That can take a while. It stays small after that (it does not keep full history).

## Day-to-day

Useful commands on the VPS:

```bash
cd /opt/gopool
docker compose ps
docker compose logs -f gopool
docker compose logs -f gopool-validator
```

In the operator dashboard you can change the pool name and fee, turn on Telegram or Discord alerts, and watch whether the validator is elected.

Delegators stake to **your validator address** (the Nimiq one from setup), not to a GoPool account.

## If something looks stuck

| What you see | What to do |
| --- | --- |
| Setup page will not load over HTTPS | DNS is not pointing where you think, or the reverse proxy is not forwarding to `http://VPS:80`. |
| Waiting for NIM / readiness error | The 101,000 NIM deposit is not there yet. Send it, then wait. |
| Dashboard says “activating” | Give it a minute. Config is applying. Do **not** restart Compose for this. |
| Validator still syncing | First start downloads the chain. Watch `docker compose logs -f gopool-validator`. |

## Extra options

Add these to the same install command:

- `--dir /path` — install somewhere other than `/opt/gopool`
- `--tls auto` — this machine gets HTTPS (Let's Encrypt)
- `--tls proxy` — HTTP on port 80, for Nginx Proxy Manager / Traefik / Cloudflare in front

Running several machines as a Docker Swarm cluster: [deployments/SWARM.md](deployments/SWARM.md).

Building or hacking on GoPool itself: [CONTRIBUTING.md](CONTRIBUTING.md).
