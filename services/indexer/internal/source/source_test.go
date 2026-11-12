package source

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := New(srv.URL)
	for _, call := range []func() error{
		func() error { _, err := c.Block(context.Background(), 1); return err },
		func() error { _, err := c.Final(context.Background()); return err },
	} {
		var rl *RateLimited
		if err := call(); !errors.As(err, &rl) || rl.Wait != 7*time.Second {
			t.Fatalf("want RateLimited 7s, got %v", err)
		}
	}
}

func TestRetryAfter(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for v, want := range map[string]time.Duration{
		"30": 30 * time.Second, "": 0, "junk": 0, "-5": 0,
		now.Add(90 * time.Second).Format(http.TimeFormat): 90 * time.Second,
		now.Add(-time.Minute).Format(http.TimeFormat):     0,
	} {
		if got := retryAfter(v, now); got != want {
			t.Errorf("retryAfter(%q) = %s, want %s", v, got, want)
		}
	}
}
