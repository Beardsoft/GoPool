package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	nimiq "github.com/NimMiniApps/nimiq-go"
)

const (
	sessionCookieName = "gopool_session"
	sessionTTL        = 24 * time.Hour
	nonceTTL          = 5 * time.Minute
)

// nonceStore holds outstanding login challenges. In-memory and single-use:
// fine for one API process; if this ever runs behind a load balancer with
// multiple API instances, move it to the shared sqlite file instead.
type nonceStore struct {
	mu   sync.Mutex
	data map[string]nonceEntry
}

type nonceEntry struct {
	challenge string
	expires   time.Time
}

func newNonceStore() *nonceStore {
	return &nonceStore{data: make(map[string]nonceEntry)}
}

func (s *nonceStore) issue() (nonce, challenge string) {
	var buf [16]byte
	_, _ = rand.Read(buf[:])
	nonce = base64.RawURLEncoding.EncodeToString(buf[:])
	challenge = fmt.Sprintf("GoPool login\nnonce: %s\nexpires: %s", nonce, time.Now().Add(nonceTTL).Format(time.RFC3339))

	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[nonce] = nonceEntry{challenge: challenge, expires: time.Now().Add(nonceTTL)}
	return nonce, challenge
}

// consume returns the challenge text for nonce and deletes it — single use,
// whether or not verification against it later succeeds.
func (s *nonceStore) consume(nonce string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.data[nonce]
	delete(s.data, nonce)
	if !ok || time.Now().After(entry.expires) {
		return "", false
	}
	return entry.challenge, true
}

type challengeResponse struct {
	Nonce     string `json:"nonce"`
	Challenge string `json:"challenge"`
}

type verifyRequest struct {
	Nonce     string `json:"nonce"`
	Address   string `json:"address"`
	PublicKey string `json:"public_key"`
	Signature string `json:"signature"`
}

func (a *API) registerAuthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/challenge", a.handleAuthChallenge)
	mux.HandleFunc("POST /api/auth/verify", a.handleAuthVerify)
	mux.HandleFunc("GET /api/session", a.requireSession(a.handleSession))
	mux.HandleFunc("POST /api/auth/logout", a.handleAuthLogout)
}

func (a *API) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type sessionResponse struct {
	Address  string `json:"address"`
	Operator bool   `json:"operator"`
}

func (a *API) handleSession(w http.ResponseWriter, r *http.Request) {
	addr, _ := addressFromContext(r.Context())
	writeJSON(w, http.StatusOK, sessionResponse{
		Address:  addr.String(),
		Operator: addr.String() == a.cfg.ValidatorAddress || a.isOperatorAllowed(addr.String()),
	})
}

func (a *API) handleAuthChallenge(w http.ResponseWriter, r *http.Request) {
	nonce, challenge := a.nonces.issue()
	writeJSON(w, http.StatusOK, challengeResponse{Nonce: nonce, Challenge: challenge})
}

func (a *API) handleAuthVerify(w http.ResponseWriter, r *http.Request) {
	var req verifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request body")
		return
	}

	challenge, ok := a.nonces.consume(req.Nonce)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown, expired, or already-used nonce")
		return
	}

	addr, err := nimiq.ParseAddress(req.Address)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid address")
		return
	}
	pub, err := nimiq.ParsePublicKey(req.PublicKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid public key")
		return
	}
	sig, err := nimiq.ParseSignature(req.Signature)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid signature")
		return
	}
	if err := nimiq.VerifyMessageFrom(addr, pub, []byte(challenge), sig); err != nil {
		writeError(w, http.StatusUnauthorized, "signature verification failed")
		return
	}

	secure := r.TLS != nil
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: a.issueSession(addr), Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secure, MaxAge: int(sessionTTL.Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]string{"address": addr.String()})
}

// issueSession builds an HMAC-signed, self-contained session token:
// address.expiry.signature. No server-side session store — verifying is
// just recomputing the HMAC, which is why SessionSecret must be kept private
// and rotating it invalidates every outstanding session at once.
func (a *API) issueSession(addr nimiq.Address) string {
	exp := time.Now().Add(sessionTTL).Unix()
	payload := addr.Compact() + "." + strconv.FormatInt(exp, 10)
	mac := hmac.New(sha256.New, []byte(a.cfg.SessionSecret))
	mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (a *API) parseSession(token string) (nimiq.Address, error) {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		return nimiq.Address{}, fmt.Errorf("api: malformed session token")
	}
	payload := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, []byte(a.cfg.SessionSecret))
	mac.Write([]byte(payload))
	want := mac.Sum(nil)
	got, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(want, got) {
		return nimiq.Address{}, fmt.Errorf("api: invalid session signature")
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return nimiq.Address{}, fmt.Errorf("api: session expired")
	}
	return nimiq.ParseAddress(parts[0])
}

type contextKey int

const addressContextKey contextKey = 0

func addressFromContext(ctx context.Context) (nimiq.Address, bool) {
	addr, ok := ctx.Value(addressContextKey).(nimiq.Address)
	return addr, ok
}

// requireSession 401s unless the request carries a valid, unexpired session
// cookie, and makes the authenticated address available to next via context.
func (a *API) requireSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "not logged in")
			return
		}
		addr, err := a.parseSession(cookie.Value)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "session invalid or expired")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), addressContextKey, addr)))
	}
}
