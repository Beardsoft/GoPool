package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"time"
)

const setupCookieName = "gopool_setup"
const setupSessionTTL = 30 * time.Minute

type setupContextKey int

const setupSessionContextKey setupContextKey = 0

func (a *API) registerSetupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/setup/session", a.handleSetupSession)
	mux.HandleFunc("GET /api/setup/status", a.requireSetup(a.handleSetupStatus))
	mux.HandleFunc("POST /api/setup/check-rpc", a.requireSetup(a.handleSetupCheckRPC))
	mux.HandleFunc("POST /api/setup/validate", a.requireSetup(a.handleSetupValidate))
	mux.HandleFunc("POST /api/setup/complete", a.requireSetup(a.handleSetupComplete))
}

func setupRemoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func requestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (a *API) handleSetupSession(w http.ResponseWriter, r *http.Request) {
	a.setupMu.Lock()
	complete := a.setupComplete
	now := time.Now()
	ip := setupRemoteIP(r)
	recent := a.setupFailures[ip][:0]
	for _, attempted := range a.setupFailures[ip] {
		if now.Sub(attempted) < time.Minute {
			recent = append(recent, attempted)
		}
	}
	a.setupFailures[ip] = recent
	a.setupMu.Unlock()
	if complete {
		writeAPIError(w, http.StatusNotFound, "setup_complete", "setup is complete")
		return
	}
	if len(recent) >= 5 {
		writeAPIError(w, http.StatusTooManyRequests, "rate_limited", "too many setup attempts")
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	if decodeJSON(r.Body, &body) != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_body", "invalid json")
		return
	}
	sum := sha256.Sum256([]byte(body.Token))
	provided := hex.EncodeToString(sum[:])
	if len(provided) != len(a.setupTokenHash) || subtle.ConstantTimeCompare([]byte(provided), []byte(a.setupTokenHash)) != 1 {
		a.setupMu.Lock()
		a.setupFailures[ip] = append(a.setupFailures[ip], now)
		a.setupMu.Unlock()
		writeAPIError(w, http.StatusUnauthorized, "invalid_token", "invalid setup token")
		return
	}
	var tokenBytes [32]byte
	if _, err := rand.Read(tokenBytes[:]); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "random_failed", "creating setup session")
		return
	}
	session := base64.RawURLEncoding.EncodeToString(tokenBytes[:])
	a.setupMu.Lock()
	a.setupSessions[session] = now.Add(setupSessionTTL)
	delete(a.setupFailures, ip)
	a.setupMu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: setupCookieName, Value: session, Path: "/api/setup", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: requestIsHTTPS(r), MaxAge: int(setupSessionTTL.Seconds())})
	writeJSON(w, http.StatusOK, map[string]any{"expires_in": int(setupSessionTTL.Seconds())})
}

func (a *API) requireSetup(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.setupMu.Lock()
		if a.setupComplete {
			a.setupMu.Unlock()
			writeAPIError(w, http.StatusNotFound, "setup_complete", "setup is complete")
			return
		}
		cookie, err := r.Cookie(setupCookieName)
		expires, ok := time.Time{}, false
		if err == nil {
			expires, ok = a.setupSessions[cookie.Value]
		}
		if !ok || time.Now().After(expires) {
			if err == nil {
				delete(a.setupSessions, cookie.Value)
			}
			a.setupMu.Unlock()
			writeAPIError(w, http.StatusUnauthorized, "setup_session_required", "setup session required")
			return
		}
		a.setupMu.Unlock()
		next(w, r.WithContext(context.WithValue(r.Context(), setupSessionContextKey, true)))
	}
}
