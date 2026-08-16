// Package api serves GoPool's read/operator HTTP API. It never holds the
// validator's private key — see cmd/api/main.go and internal/pool's
// ProcessRequestedActions for why.
package api

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/NimMiniApps/nimiq-go/rpc"

	"github.com/Beardsoft/GoPool/internal/config"
	"github.com/Beardsoft/GoPool/internal/configstore"
	"github.com/Beardsoft/GoPool/internal/db"
)

// API holds the dependencies every handler needs.
type API struct {
	queries        *db.Queries
	cfg            *config.Config
	rpc            *rpc.Client
	nonces         *nonceStore
	alertTestMu    sync.Mutex
	alertTestAt    map[string]time.Time
	configStore    *configstore.Store
	setupOnly      bool
	setupComplete  bool
	setupTokenHash string
	setupMu        sync.Mutex
	setupSessions  map[string]time.Time
	setupFailures  map[string][]time.Time
}

type Option func(*API)

func WithConfigStore(store *configstore.Store) Option { return func(a *API) { a.configStore = store } }
func WithSetup(tokenHash string, store *configstore.Store) Option {
	return func(a *API) { a.setupOnly = true; a.setupTokenHash = tokenHash; a.configStore = store }
}

// New builds an API. cfg may be nil in tests that don't exercise auth or the
// operator health endpoint; rpc may be nil in tests that don't exercise
// operator health.
func New(cfg *config.Config, q *db.Queries, rpcClient *rpc.Client, options ...Option) *API {
	a := &API{queries: q, cfg: cfg, rpc: rpcClient, nonces: newNonceStore(), alertTestAt: make(map[string]time.Time), setupSessions: make(map[string]time.Time), setupFailures: make(map[string][]time.Time)}
	for _, option := range options {
		option(a)
	}
	return a
}

// Mux builds the HTTP routes. Route registration for auth/operator handlers
// is added in later tasks; this file owns the mux itself so every task edits
// only the routes it adds, not a shared switch statement.
func (a *API) Mux() http.Handler {
	mux := http.NewServeMux()
	if a.setupOnly {
		a.registerSetupRoutes(mux)
		a.registerStaticRoutes(mux)
		return mux
	}
	mux.HandleFunc("GET /api/pool", a.handlePool)
	mux.HandleFunc("GET /api/pool/rewards", a.handlePoolRewards)
	a.registerEpochRoutes(mux)
	a.registerStakerRoutes(mux)
	a.registerAuthRoutes(mux)
	a.registerOperatorRoutes(mux)
	a.registerOperatorSettingsRoutes(mux)
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
