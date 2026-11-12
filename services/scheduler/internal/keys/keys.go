// Package keys checks on chain that the executor key is still on a user's account, and its base balance.
package keys

import (
	"context"
	"errors"
	"strings"

	"github.com/Debanjannnn/nearly/internal/shared/near"
)

type Checker struct {
	RPC         *near.Client
	ExecutorKey string // "ed25519:..."
	Token       string // base token contract
}

// Base is the account's ft_balance_of of the base token.
func (c *Checker) Base(ctx context.Context, account string) (string, error) {
	var b string
	return b, c.RPC.View(ctx, c.Token, "ft_balance_of", map[string]string{"account_id": account}, &b)
}

// Active is false once the owner deleted the executor key (or the account is gone).
func (c *Checker) Active(ctx context.Context, account string) (bool, error) {
	_, err := c.RPC.ViewAccessKey(ctx, account, c.ExecutorKey)
	if missing(err) {
		return false, nil
	}
	return err == nil, err
}

func missing(err error) bool {
	var re *near.RPCError
	if errors.As(err, &re) {
		return re.Cause.Name == "UNKNOWN_ACCESS_KEY" || re.Cause.Name == "UNKNOWN_ACCOUNT"
	}
	// view_access_key errors also arrive inside result as a plain string
	return err != nil && (strings.Contains(err.Error(), "does not exist") || strings.Contains(err.Error(), "UNKNOWN_ACCESS_KEY"))
}
