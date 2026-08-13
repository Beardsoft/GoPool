package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerMetricsAndHealthz(t *testing.T) {
	ChainHead.Set(42)

	srv := httptest.NewServer(Handler())
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics status %d", res.StatusCode)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "gopool_chain_head") {
		t.Fatalf("metrics body missing gopool_chain_head:\n%s", text)
	}

	hz, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer hz.Body.Close()
	if hz.StatusCode != http.StatusOK {
		t.Fatalf("GET /healthz status %d", hz.StatusCode)
	}
}
