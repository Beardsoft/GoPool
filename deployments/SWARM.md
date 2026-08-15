# Docker Swarm deployment

GoPool uses three separate secrets. Only the daemon receives the validator key; only the API receives setup and session secrets. Both services share SQLite and a configuration volume, mounted read-only in the daemon and writable in the API.

```bash
openssl rand -hex 32 | docker secret create gopool_setup_token -
openssl rand -hex 32 | docker secret create gopool_session_secret -
printf '%s' '<validator-private-key-hex>' | docker secret create gopool_validator_key -
docker stack deploy -c deployments/docker-stack.yml gopool
```

Open the configured Traefik hostname, enter the setup token value, complete the assistant, then restart both services so the daemon reads the written configuration:

```bash
docker service update --force gopool_gopool
docker service update --force gopool_gopool-api
docker service logs --tail 100 gopool_gopool
```

Readiness is complete only when the daemon heartbeat reports the new configuration hash and derived validator address. A written revision is not active merely because the API accepted it.

## Rotation and recovery

- Rotate a Docker secret by creating a new uniquely named secret, updating the stack reference, deploying, then removing the old secret after both services converge.
- The setup token is unusable after setup completion and may be removed from the API service in a follow-up stack revision.
- Restore a prior non-secret revision from Operator → Settings, restart both services, and wait for the matching heartbeat.
- If readiness fails, do not retry on-chain actions. Check the daemon logs, RPC/network pairing, validator-address mismatch, key-file permissions, and the pending configuration hash first.
- SQLite and the configuration volume must stay on the manager selected by the placement constraint.
