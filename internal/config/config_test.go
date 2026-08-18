package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateAPI(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"all set", Config{APIAddr: ":8080", SessionSecret: "s3cr3t", ValidatorAddress: "NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E"}, false},
		{"missing api_addr", Config{SessionSecret: "s3cr3t", ValidatorAddress: "NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E"}, true},
		{"missing session_secret", Config{APIAddr: ":8080", ValidatorAddress: "NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E"}, true},
		{"missing validator_address", Config{APIAddr: ":8080", SessionSecret: "s3cr3t"}, true},
		{"invalid validator_address", Config{APIAddr: ":8080", SessionSecret: "s3cr3t", ValidatorAddress: "not-an-address"}, true},
		{"whitespace session_secret", Config{APIAddr: ":8080", SessionSecret: "   ", ValidatorAddress: "NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E"}, true},
	}
	for _, c := range cases {
		err := ValidateAPI(&c.cfg)
		if (err != nil) != c.wantErr {
			t.Errorf("%s: ValidateAPI() error = %v, wantErr %v", c.name, err, c.wantErr)
		}
	}
}

func TestLoadOptionalDoesNotMaterializeSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"rpc_url":"https://rpc.example","network":"main","private_key":"private-value","session_secret":"session-value"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, configured, err := LoadOptional(path)
	if err != nil || !configured {
		t.Fatalf("configured=%v err=%v", configured, err)
	}
	if cfg.PrivateKey != "" || cfg.SessionSecret != "" {
		t.Fatalf("API load materialized a secret: %+v", cfg)
	}
}

func TestLoadOptionalKeepsAlertSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	seed := `{"rpc_url":"https://rpc.example","network":"main","alert_telegram_token":"tg-value","alert_webhook_url":"https://discord.com/api/webhooks/1/wh-value","stuck_payout_epochs":9}`
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, configured, err := LoadOptional(path)
	if err != nil || !configured {
		t.Fatalf("configured=%v err=%v", configured, err)
	}
	if cfg.AlertTelegramToken != "tg-value" || cfg.AlertWebhookURL != "https://discord.com/api/webhooks/1/wh-value" || cfg.StuckPayoutEpochs != 9 {
		t.Fatalf("alert secrets lost on load: %+v", cfg.AlertSecrets())
	}
}

func TestConfigHashChangesWithSecrets(t *testing.T) {
	editable := Editable{PoolName: "GoPool"}
	base := ConfigHash(editable, AlertSecrets{})
	if ConfigHash(editable, AlertSecrets{}) != base {
		t.Fatal("hash must be stable")
	}
	if ConfigHash(editable, AlertSecrets{TelegramToken: "x"}) == base {
		t.Fatal("secret change must change the hash")
	}
}

func TestRedactedConfigNeverContainsSecrets(t *testing.T) {
	cfg := Config{PrivateKey: "private-value-7f8c", SessionSecret: "session-value-5a2d", AlertTelegramToken: "telegram-value-9b1e", AlertWebhookURL: "https://discord.com/api/webhooks/1/webhook-value-77aa"}
	got, _ := json.Marshal(Redact(&cfg))
	for _, secret := range []string{cfg.PrivateKey, cfg.SessionSecret, cfg.AlertTelegramToken, "webhook-value-77aa"} {
		if strings.Contains(string(got), secret) {
			t.Fatal(string(got))
		}
	}
	if !Redact(&cfg).Secrets.WebhookURL {
		t.Fatalf("webhook_url presence not reported: %+v", Redact(&cfg).Secrets)
	}
}

func TestValidateAPICanonicalizesAddress(t *testing.T) {
	const canonical = "NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E"
	const operator = "NQ17 VERV F3MQ 283T NRSR FPJG 55BJ PMHC N8MD"
	cfg := Config{
		APIAddr:           ":8080",
		SessionSecret:     "s3cr3t",
		ValidatorAddress:  "nq20tsb0dfsmuh9c15gqgagjtte4d3ma859e",
		OperatorAddresses: "nq17vervf3mq283tnrsrfpjg55bjpmhcn8md",
	}
	if err := ValidateAPI(&cfg); err != nil {
		t.Fatalf("ValidateAPI() error = %v, want nil", err)
	}
	if cfg.ValidatorAddress != canonical {
		t.Errorf("ValidatorAddress = %q, want canonical %q", cfg.ValidatorAddress, canonical)
	}
	if cfg.OperatorAddresses != operator {
		t.Errorf("OperatorAddresses = %q, want canonical %q", cfg.OperatorAddresses, operator)
	}
}

func TestValidateStillIgnoresMetricsAddr(t *testing.T) {
	cfg := Config{
		PayoutMode:        "delegate",
		PoolFeePercentage: 0.01,
		MetricsAddr:       "",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}
