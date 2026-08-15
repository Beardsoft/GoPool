package api

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/Beardsoft/GoPool/internal/config"
	"github.com/Beardsoft/GoPool/internal/configstore"
)

func TestSettingsSaveRequiresCurrentHash(t *testing.T) {
	a, cookie := configuredOperatorAPI(t)
	store := configstore.New(filepath.Join(t.TempDir(), "config.json"), a.queries)
	a.configStore = store
	settings := config.Editable{RPCURL: "https://rpc-testnet.nimiqscan.com", Network: "test-albatross", PoolFeeWallet: testAddr, PoolFeePercentage: .01, PayoutMode: "delegate", MinPayoutLuna: 100_000, APIAddr: ":8080", ValidatorAddress: testAddr, PoolName: "GoPool"}
	if _, err := store.Save(t.Context(), testAddr, "", settings); err != nil {
		t.Fatal(err)
	}
	rec := putJSON(t, a.Mux(), "/api/operator/settings", map[string]any{"expected_hash": "stale", "settings": settings}, cookie)
	if rec.Code != http.StatusConflict {
		t.Fatalf("%d: %s", rec.Code, rec.Body.String())
	}
}
