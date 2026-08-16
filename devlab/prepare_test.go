package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/NimMiniApps/nimiq-go/signer"
)

const seed1ValidatorKey = "6927eb8de74e8ea06a8afae5a66db176a7031f742b656651ac53bddb8a4ad3f3"

func runPrepare(t *testing.T, root string) {
	t.Helper()
	cmd := exec.Command("bash", "prepare.sh")
	cmd.Env = append(os.Environ(), "GOPOOL_DEV_ROOT="+root)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("prepare.sh failed: %v\n%s", err, output)
	}
}

func TestPrepareCreatesMissingDevnetFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "data"), 0o555); err != nil {
		t.Fatal(err)
	}
	runPrepare(t, root)

	checks := map[string]string{
		filepath.Join(root, ".secrets", "validator-key"):                   seed1ValidatorKey + "\n",
		filepath.Join(root, "devlab", ".runtime", "config", "config.json"): "",
	}
	for path, want := range checks {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if want != "" && string(got) != want {
			t.Fatalf("%s = %q, want %q", path, got, want)
		}
		if len(got) == 0 {
			t.Fatalf("%s is empty", path)
		}
	}

	for _, name := range []string{"setup-token", "session-secret"} {
		path := filepath.Join(root, ".secrets", name)
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if len(got) != 65 || got[64] != '\n' {
			t.Fatalf("%s has unexpected token length %d", path, len(got))
		}
	}

	for _, name := range []string{"validator-key", "setup-token", "session-secret"} {
		path := filepath.Join(root, ".secrets", name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("%s mode = %o, want 600", path, got)
		}
	}
}

func TestDevnetValidatorKeyMatchesConfiguredValidator(t *testing.T) {
	key, err := signer.ParsePrivateKeyHex(seed1ValidatorKey)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := key.Address().String(), "NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E"; got != want {
		t.Fatalf("validator key derives %s, want %s", got, want)
	}
}

func TestPrepareMigratesConsensusSigningKey(t *testing.T) {
	root := t.TempDir()
	secretDir := filepath.Join(root, ".secrets")
	if err := os.MkdirAll(secretDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(secretDir, "validator-key")
	if err := os.WriteFile(path, []byte("041580cc67e66e9e08b68fd9e4c9deb68737168fbe7488de2638c2e906c2f5ad\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	runPrepare(t, root)

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := seed1ValidatorKey + "\n"; string(got) != want {
		t.Fatalf("validator key = %q, want %q", got, want)
	}
}

func TestPreparePreservesExistingDevnetFiles(t *testing.T) {
	root := t.TempDir()
	secretDir := filepath.Join(root, ".secrets")
	configDir := filepath.Join(root, "devlab", ".runtime", "config")
	if err := os.MkdirAll(secretDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}

	existing := map[string]string{
		filepath.Join(secretDir, "validator-key"):  "custom-validator-key\n",
		filepath.Join(secretDir, "setup-token"):    "custom-setup-token\n",
		filepath.Join(secretDir, "session-secret"): "custom-session-secret\n",
		filepath.Join(configDir, "config.json"):    "{\"pool_name\":\"Custom\"}\n",
	}
	for path, contents := range existing {
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	runPrepare(t, root)

	for path, want := range existing {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if string(got) != want {
			t.Fatalf("%s was overwritten: got %q, want %q", path, got, want)
		}
	}
}
