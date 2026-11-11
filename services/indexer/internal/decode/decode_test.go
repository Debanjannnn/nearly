package decode

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/Debanjannnn/nearly/services/indexer/internal/source"
)

func TestEvents(t *testing.T) {
	b, err := os.ReadFile("testdata/block.json")
	if err != nil {
		t.Fatal(err)
	}
	var blk source.Block
	if err := json.Unmarshal(b, &blk); err != nil {
		t.Fatal(err)
	}
	got := Events(&blk, "factory.testnet")
	// failed receipt, foreign account, other standard, wrong version and junk logs are all dropped
	want := []struct {
		receipt string
		idx     int
		event   string
	}{{"RcptDeposit", 0, "deposit"}, {"RcptStep", 2, "step"}, {"RcptRevoke", 0, "revoke"}}
	if len(got) != len(want) {
		t.Fatalf("got %d events: %+v", len(got), got)
	}
	for i, w := range want {
		e := got[i]
		if e.ReceiptID != w.receipt || e.LogIndex != w.idx || e.Event != w.event || e.Account != "alice-testnet.factory.testnet" {
			t.Fatalf("event %d = %+v, want %+v", i, e, w)
		}
	}
	if got[0].Height != 277250759 || got[0].TxHash != "6xdepTx" || got[0].Time.Year() != 2026 {
		t.Fatalf("meta %+v", got[0])
	}
	var d struct{ Amount string }
	if json.Unmarshal(got[0].Data, &d); d.Amount != "1000000000" {
		t.Fatalf("data %s", got[0].Data)
	}
}
