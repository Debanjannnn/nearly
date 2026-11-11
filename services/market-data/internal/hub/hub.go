// Package hub fans poller snapshots out to WebSocket clients ({op:"sub"|"unsub", topics:[...]}).
package hub

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	sendBuffer = 16 // a client this far behind is dropped
	pingEvery  = 20 * time.Second
	writeWait  = 10 * time.Second
)

var Topics = map[string]bool{"rates": true, "prices": true}

// client.drop must be safe to call repeatedly and must not call back into the hub.
type client struct {
	send chan []byte
	drop func()
}

type Hub struct {
	// OriginPatterns are host[:port] globs allowed to open the socket.
	OriginPatterns []string
	mu             sync.Mutex
	subs           map[string]map[*client]struct{}
	last           map[string][]byte // replayed to new subscribers
}

func New() *Hub { return &Hub{subs: map[string]map[*client]struct{}{}, last: map[string][]byte{}} }

func (h *Hub) Publish(topic string, v any) {
	b, err := json.Marshal(map[string]any{"topic": topic, "data": v})
	if err != nil {
		slog.Error("hub marshal", "topic", topic, "err", err)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.last[topic] = b
	for c := range h.subs[topic] {
		c.push(b)
	}
}

func (h *Hub) sub(c *client, t string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs[t] == nil {
		h.subs[t] = map[*client]struct{}{}
	}
	h.subs[t][c] = struct{}{}
	if b := h.last[t]; b != nil {
		c.push(b)
	}
}

func (h *Hub) unsub(c *client, t string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subs[t], c)
}

func (c *client) push(b []byte) {
	select {
	case c.send <- b:
	default:
		c.drop()
	}
}

// Serve upgrades to WebSocket and runs the sub/unsub protocol until the client leaves or falls behind.
func (h *Hub) Serve(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: h.OriginPatterns})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(4096)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	c := &client{send: make(chan []byte, sendBuffer), drop: cancel}
	defer func() {
		for t := range Topics {
			h.unsub(c, t)
		}
	}()

	go func() {
		ping := time.NewTicker(pingEvery)
		defer ping.Stop()
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case b := <-c.send:
				wctx, wc := context.WithTimeout(ctx, writeWait)
				err := conn.Write(wctx, websocket.MessageText, b)
				wc()
				if err != nil {
					return
				}
			case <-ping.C:
				pctx, pc := context.WithTimeout(ctx, writeWait)
				err := conn.Ping(pctx)
				pc()
				if err != nil {
					return
				}
			}
		}
	}()

	reply := func(v any) {
		b, _ := json.Marshal(v)
		c.push(b)
	}
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var msg struct {
			Op     string   `json:"op"`
			Topics []string `json:"topics"`
		}
		if json.Unmarshal(data, &msg) != nil || (msg.Op != "sub" && msg.Op != "unsub") {
			reply(map[string]string{"error": "bad_request"})
			continue
		}
		for _, t := range msg.Topics {
			switch {
			case !Topics[t]:
				reply(map[string]string{"error": "bad_topic", "topic": t})
			case msg.Op == "sub":
				h.sub(c, t)
			default:
				h.unsub(c, t)
			}
		}
	}
}
