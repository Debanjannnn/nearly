package funds

import (
	"math/big"
	"testing"
)

func TestFree(t *testing.T) {
	got := Free(map[string]string{"usdc": "100", "wnear": "5", "x": "bad"}, map[string]*big.Int{"usdc": big.NewInt(30), "wnear": big.NewInt(9)})
	if got["usdc"] != "70" || got["wnear"] != "0" || got["x"] != "0" {
		t.Fatal(got)
	}
}
