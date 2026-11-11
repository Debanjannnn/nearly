package hub

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestServe(t *testing.T) {
	h := New()
	srv := httptest.NewServer(http.HandlerFunc(h.Serve))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	read := func() string {
		_, b, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	write := func(s string) { conn.Write(ctx, websocket.MessageText, []byte(s)) }

	h.Publish("rates", []int{1})
	write(`{"op":"sub","topics":["rates","nope"]}`)
	if got := read(); got != `{"data":[1],"topic":"rates"}` {
		t.Fatal("replay", got)
	}
	if got := read(); got != `{"error":"bad_topic","topic":"nope"}` {
		t.Fatal(got)
	}
	h.Publish("prices", 0) // not subscribed
	h.Publish("rates", []int{2})
	if got := read(); got != `{"data":[2],"topic":"rates"}` {
		t.Fatal("live", got)
	}
	write(`junk`)
	if got := read(); got != `{"error":"bad_request"}` {
		t.Fatal(got)
	}
}

func TestSlowClientDropped(t *testing.T) {
	h := New()
	var dropped atomic.Bool
	c := &client{send: make(chan []byte, 1), drop: func() { dropped.Store(true) }}
	h.sub(c, "rates")
	h.Publish("rates", 1)
	h.Publish("rates", 2)
	if !dropped.Load() {
		t.Fatal("slow client kept")
	}
	h.unsub(c, "rates")
	if len(h.subs["rates"]) != 0 {
		t.Fatal("still subscribed")
	}
}
