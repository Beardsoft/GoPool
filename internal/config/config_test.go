package config

import "testing"

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

func TestValidateAPICanonicalizesAddress(t *testing.T) {
	const canonical = "NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E"
	cfg := Config{
		APIAddr:          ":8080",
		SessionSecret:    "s3cr3t",
		ValidatorAddress: "nq20tsb0dfsmuh9c15gqgagjtte4d3ma859e",
	}
	if err := ValidateAPI(&cfg); err != nil {
		t.Fatalf("ValidateAPI() error = %v, want nil", err)
	}
	if cfg.ValidatorAddress != canonical {
		t.Errorf("ValidatorAddress = %q, want canonical %q", cfg.ValidatorAddress, canonical)
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
