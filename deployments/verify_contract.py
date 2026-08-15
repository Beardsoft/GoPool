import json
import subprocess


def rendered(path, *extra):
    command = ["docker", "compose", "-f", path]
    for item in extra:
        command.extend(["-f", item])
    command.extend(["config", "--format", "json"])
    return json.loads(subprocess.check_output(command, text=True))


def assert_contract(services):
    api = json.dumps(services["gopool-api"])
    daemon = json.dumps(services["gopool"])
    assert "POOL_PRIVATE_KEY" not in api and "gopool_validator_key" not in api
    assert "POOL_PRIVATE_KEY_FILE" in daemon and "gopool_validator_key" in daemon
    assert "gopool_setup_token" in api and "gopool_session_secret" in api
    api_config = next(v for v in services["gopool-api"].get("volumes", []) if v.get("target") == "/root/config")
    daemon_config = next(v for v in services["gopool"].get("volumes", []) if v.get("target") == "/root/config")
    assert not api_config.get("read_only", False) and daemon_config.get("read_only", False)


assert_contract(rendered("deployments/docker-compose.yml")["services"])
assert_contract(rendered("devlab/docker-compose.yaml", "devlab/docker-compose.pool.yml")["services"])
print("deployment contract verified")
