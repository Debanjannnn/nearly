package executor

import (
	"encoding/json"
	"log/slog"
	"regexp"
	"strings"
)

// Revoked is the step error when the account's executor key is gone on chain.
const Revoked = "executor_revoked"

var panicRe = regexp.MustCompile(`Smart contract panicked: ((?:[^"\\]|\\.)*)`)

// Clean maps a raw executor step error to a short user message; raw NEAR RPC json never reaches users.
func Clean(raw string) string {
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(raw, s) {
				return true
			}
		}
		return false
	}
	switch {
	case has("AccessKeyNotFound", "UNKNOWN_ACCESS_KEY"):
		return Revoked
	case has("NotEnoughAllowance"):
		return "executor key gas allowance used up"
	case has("InvalidAccessKeyError"):
		return "executor key rejected"
	case has("LackBalanceForState"):
		return "not enough NEAR for storage"
	case has("NotEnoughBalance"):
		return "not enough NEAR for gas"
	case has("GasExceeded", "Exceeded the prepaid gas"):
		return "ran out of gas"
	}
	if m := panicRe.FindStringSubmatch(raw); m != nil {
		var msg string
		if json.Unmarshal([]byte(`"`+m[1]+`"`), &msg) != nil {
			msg = m[1]
		}
		return "contract: " + clip(strings.TrimSpace(msg))
	}
	switch {
	case has("outcome unknown"):
		return "transaction outcome unknown, check the explorer"
	case has("InvalidNonce", "nonce retries"):
		return "nonce conflict, try again"
	case has("Expired"):
		return "transaction expired, try again"
	case strings.HasPrefix(raw, "rpc transport"):
		return "rpc_unavailable"
	case has("{"):
		return "transaction failed on chain"
	}
	return clip(raw)
}

func clip(s string) string {
	if r := []rune(s); len(r) > 160 {
		return string(r[:160]) + "…"
	}
	return s
}

// Tidy cleans every step error in place, logging the raw one; true = the executor key is gone.
func Tidy(execID string, results []Result) (revoked bool) {
	for i := range results {
		r := &results[i]
		if r.Error == "" || r.Error == "skipped" {
			continue
		}
		slog.Warn("step failed", "execution", execID, "index", r.Index, "err", r.Error)
		r.Error = Clean(r.Error)
		revoked = revoked || r.Error == Revoked
	}
	return
}
