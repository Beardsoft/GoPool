// Package api serves GoPool's read/operator HTTP API. It never holds the
// validator's private key — see cmd/api/main.go and internal/pool's
// ProcessRequestedActions for why.
package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/NimMiniApps/nimiq-go/rpc"

	"github.com/Beardsoft/GoPool/internal/config"
	"github.com/Beardsoft/GoPool/internal/db"
)

// API holds the dependencies every handler needs.
type API struct {
	queries *db.Queries
	cfg     *config.Config
	rpc     *rpc.Client
	nonces  *nonceStore
}

// New builds an API. cfg may be nil in tests that don't exercise auth or the
// operator health endpoint; rpc may be nil in tests that don't exercise
// operator health.
func New(cfg *config.Config, q *db.Queries, rpcClient *rpc.Client) *API {
	return &API{queries: q, cfg: cfg, rpc: rpcClient, nonces: newNonceStore()}
}

// Mux builds the HTTP routes. Route registration for auth/operator handlers
// is added in later tasks; this file owns the mux itself so every task edits
// only the routes it adds, not a shared switch statement.
func (a *API) Mux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/pool", a.handlePool)
	a.registerEpochRoutes(mux)
	a.registerStakerRoutes(mux)
	a.registerAuthRoutes(mux)
	a.registerOperatorRoutes(mux)
	a.registerStaticRoutes(mux)
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeJSON(r io.Reader, v any) error {
	return json.NewDecoder(r).Decode(v)
}
