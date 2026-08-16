package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/Beardsoft/GoPool/internal/configstore"
)

func TestSetupTokenExchangeDisabledAfterCompletion(t *testing.T) {
	sum := sha256.Sum256([]byte("bootstrap"))
	q := newTestDB(t)
	store := configstore.New(filepath.Join(t.TempDir(), "config.json"), q)
	a := New(nil, q, nil, WithSetup(hex.EncodeToString(sum[:]), store))
	a.setupComplete = true
	rec := postJSON(t, a.Mux(), "/api/setup/session", map[string]string{"token": "bootstrap"}, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("%d", rec.Code)
	}
}
