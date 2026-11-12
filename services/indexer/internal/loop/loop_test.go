package loop

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Debanjannnn/nearly/services/indexer/internal/source"
)

func TestBackoff(t *testing.T) {
	every, boom := 5*time.Second, errors.New("boom")
	cases := []struct {
		prev time.Duration
		err  error
		want time.Duration
	}{
		{time.Minute, nil, every},
		{every, boom, 10 * time.Second},
		{90 * time.Second, boom, maxBackoff},
		{every, fmt.Errorf("tick: %w", &source.RateLimited{Wait: 42 * time.Second}), 42 * time.Second},
		{every, &source.RateLimited{}, 10 * time.Second}, // no Retry-After: plain doubling
	}
	for _, c := range cases {
		if got := backoff(c.prev, every, c.err); got != c.want {
			t.Errorf("backoff(%s, %v) = %s, want %s", c.prev, c.err, got, c.want)
		}
	}
}

func TestHealth(t *testing.T) {
	x := &Indexer{MaxLag: 100}
	check := func(want int, lag uint64) {
		t.Helper()
		w := httptest.NewRecorder()
		x.Health(w, httptest.NewRequest("GET", "/health", nil))
		var out struct{ Lag uint64 }
		json.NewDecoder(w.Body).Decode(&out)
		if w.Code != want || out.Lag != lag {
			t.Fatalf("code %d lag %d, want %d %d", w.Code, out.Lag, want, lag)
		}
	}
	check(http.StatusServiceUnavailable, 0) // final never read
	x.final.Store(1000)
	x.cursor.Store(950)
	x.seen.Store(time.Now().Unix())
	check(http.StatusOK, 50)
	x.cursor.Store(800)
	check(http.StatusServiceUnavailable, 200)
	x.cursor.Store(1000)
	x.seen.Store(time.Now().Add(-time.Hour).Unix())
	check(http.StatusServiceUnavailable, 0) // final read went stale
}
