// Package decode pulls NEP-297 "nearly" events out of a block's receipt outcomes.
package decode

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/Debanjannnn/nearly/services/indexer/internal/source"
)

const (
	prefix   = "EVENT_JSON:"
	Standard = "nearly"
	Version  = "1.0.0"
)

var kinds = map[string]bool{"deposit": true, "step": true, "withdraw": true, "revoke": true}

type Event struct {
	ReceiptID string
	LogIndex  int
	Height    uint64
	Time      time.Time
	TxHash    string
	Account   string
	Event     string
	Data      json.RawMessage
}

// Events keeps logs of successful receipts run by accounts ending in "."+factory.
// A failed receipt's logs are dropped: its state change was rolled back.
func Events(b *source.Block, factory string) []Event {
	var out []Event
	suffix := "." + factory
	h := b.Block.Header
	for _, sh := range b.Shards {
		for _, o := range sh.Outcomes {
			eo := o.ExecutionOutcome
			if !strings.HasSuffix(eo.Outcome.ExecutorID, suffix) || eo.Outcome.Status["Failure"] != nil {
				continue
			}
			for i, l := range eo.Outcome.Logs {
				raw, ok := strings.CutPrefix(l, prefix)
				if !ok {
					continue
				}
				var e struct {
					Standard string          `json:"standard"`
					Version  string          `json:"version"`
					Event    string          `json:"event"`
					Data     json.RawMessage `json:"data"`
				}
				if json.Unmarshal([]byte(raw), &e) != nil || e.Standard != Standard || e.Version != Version || !kinds[e.Event] {
					continue
				}
				out = append(out, Event{eo.ID, i, h.Height, time.Unix(0, int64(h.Timestamp)).UTC(),
					o.TxHash, eo.Outcome.ExecutorID, e.Event, e.Data})
			}
		}
	}
	return out
}
