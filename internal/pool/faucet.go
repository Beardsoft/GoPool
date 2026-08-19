package pool

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Beardsoft/GoPool/internal/logger"

	"go.uber.org/zap"
)

func fundAddress(ctx context.Context, client *http.Client, faucetURL, address string) bool {
	if faucetURL == "" || address == "" {
		return false
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	data := url.Values{}
	data.Set("address", address)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, faucetURL, strings.NewReader(data.Encode()))
	if err != nil {
		logger.Logger.Error("building faucet request", zap.Error(err))
		return false
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		logger.Logger.Error("posting to faucet", zap.Error(err))
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	if resp.StatusCode != http.StatusOK {
		logger.Logger.Error("faucet returned non-OK", zap.Int("status", resp.StatusCode))
		return false
	}
	return true
}
