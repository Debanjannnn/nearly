package market

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFreshAndQuote(t *testing.T) {
	ratesStale := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/prices":
			w.Write([]byte(`{"prices":[],"stale":false}`))
		case "/api/v1/rates":
			if ratesStale {
				w.Write([]byte(`{"rates":[],"stale":true}`))
				return
			}
			w.Write([]byte(`{"rates":[],"stale":false}`))
		case "/api/v1/quote":
			if r.URL.Query().Get("pool_id") != "1900" || r.URL.Query().Get("amount_in") != "5" {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.Write([]byte(`{"amount_out":"42"}`))
		}
	}))
	defer srv.Close()
	c := &Client{URL: srv.URL, HTTP: srv.Client()}
	ctx := context.Background()
	if err := c.Fresh(ctx); err != nil {
		t.Fatal(err)
	}
	ratesStale = true
	if err := c.Fresh(ctx); !errors.Is(err, ErrStale) {
		t.Fatalf("want ErrStale, got %v", err)
	}
	if v, err := c.Quote(ctx, 1900, "a", "b", big.NewInt(5)); err != nil || v.Int64() != 42 {
		t.Fatalf("quote %v %v", v, err)
	}
	if _, err := c.Quote(ctx, 1, "a", "b", big.NewInt(5)); err == nil {
		t.Fatal("stale pool must error")
	}
}
