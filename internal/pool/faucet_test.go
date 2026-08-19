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

	if !fundAddress(context.Background(), ts.Client(), ts.URL, "NQXX") {
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
	if fundAddress(context.Background(), ts.Client(), ts.URL, "NQXX") {
		t.Fatal("expected failure")
	}
}

func TestFundAddressTimesOut(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	client := &http.Client{Timeout: 50 * time.Millisecond}
	if fundAddress(context.Background(), client, ts.URL, "NQXX") {
		t.Fatal("expected timeout failure")
	}
}
