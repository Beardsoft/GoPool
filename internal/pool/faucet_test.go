package pool

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFundAddressPostsForm(t *testing.T) {
	var gotAddr, gotCT string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		gotAddr = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	got := fundAddress(context.Background(), ts.Client(), ts.URL, "NQXX")
	if !got.OK {
		t.Fatal("expected success")
	}
	if gotCT != "application/x-www-form-urlencoded" {
		t.Fatalf("content-type = %q", gotCT)
	}
	if gotAddr != "address=NQXX" {
		t.Fatalf("body = %q", gotAddr)
	}
}

func TestFundAddressNonOK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer ts.Close()
	got := fundAddress(context.Background(), ts.Client(), ts.URL, "NQXX")
	if got.OK {
		t.Fatal("expected failure")
	}
	if got.RetryAfter != time.Hour {
		t.Fatalf("retry = %s, want 1h", got.RetryAfter)
	}
}

func TestFundAddressTimesOut(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	client := &http.Client{Timeout: 50 * time.Millisecond}
	got := fundAddress(context.Background(), client, ts.URL, "NQXX")
	if got.OK {
		t.Fatal("expected timeout failure")
	}
}

func TestFundAddressRejectsJSONFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":false,"error":"RATE_LIMIT","wait":86324,"msg":"Easy there tiger"}`))
	}))
	defer ts.Close()
	got := fundAddress(context.Background(), ts.Client(), ts.URL, "NQXX")
	if got.OK {
		t.Fatal("HTTP 200 with success:false must not count as funded")
	}
	if !got.RateLimited {
		t.Fatal("expected rate-limited")
	}
	if got.RetryAfter != 86324*time.Second {
		t.Fatalf("retry = %s", got.RetryAfter)
	}
}

func TestFundAddressJSONSuccessWaitsADay(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer ts.Close()
	got := fundAddress(context.Background(), ts.Client(), ts.URL, "NQXX")
	if !got.OK {
		t.Fatal("expected success")
	}
	if got.RetryAfter != 24*time.Hour {
		t.Fatalf("retry = %s, want 24h so we do not hammer a once-a-day faucet", got.RetryAfter)
	}
}
