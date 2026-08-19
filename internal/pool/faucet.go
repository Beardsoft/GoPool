package pool

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Beardsoft/GoPool/internal/logger"

	"go.uber.org/zap"
)

const (
	defaultFaucetRetry     = 24 * time.Hour
	transientFaucetRetry   = time.Hour
	maxFaucetResponseBytes = 2048
)

type faucetResult struct {
	OK          bool
	RateLimited bool
	RetryAfter  time.Duration
	Message     string
}

func fundAddress(ctx context.Context, client *http.Client, faucetURL, address string) faucetResult {
	if faucetURL == "" || address == "" {
		return faucetResult{RetryAfter: defaultFaucetRetry, Message: "faucet is not configured"}
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	data := url.Values{}
	data.Set("address", address)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, faucetURL, strings.NewReader(data.Encode()))
	if err != nil {
		logger.Logger.Error("building faucet request", zap.Error(err))
		return faucetResult{RetryAfter: transientFaucetRetry, Message: err.Error()}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		logger.Logger.Error("posting to faucet", zap.Error(err))
		return faucetResult{RetryAfter: transientFaucetRetry, Message: err.Error()}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxFaucetResponseBytes))
	if resp.StatusCode != http.StatusOK {
		logger.Logger.Error("faucet returned non-OK", zap.Int("status", resp.StatusCode), zap.ByteString("body", body))
		return faucetResult{RetryAfter: transientFaucetRetry, Message: "faucet HTTP error"}
	}
	return parseFaucetBody(body)
}

func parseFaucetBody(body []byte) faucetResult {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return faucetResult{OK: true, RetryAfter: defaultFaucetRetry}
	}
	var payload struct {
		Success *bool  `json:"success"`
		Error   string `json:"error"`
		Msg     string `json:"msg"`
		Wait    int64  `json:"wait"`
	}
	if json.Unmarshal(body, &payload) != nil || payload.Success == nil {
		// Non-JSON 200 (or JSON without success) — treat as accepted request.
		return faucetResult{OK: true, RetryAfter: defaultFaucetRetry}
	}
	if *payload.Success {
		return faucetResult{OK: true, RetryAfter: defaultFaucetRetry}
	}
	retry := defaultFaucetRetry
	if payload.Wait > 0 {
		retry = time.Duration(payload.Wait) * time.Second
	}
	msg := payload.Msg
	if msg == "" {
		msg = payload.Error
	}
	return faucetResult{
		RateLimited: strings.EqualFold(payload.Error, "RATE_LIMIT"),
		RetryAfter:  retry,
		Message:     msg,
	}
}
