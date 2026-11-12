// Package chain holds the NEAR RPC reads the api needs: keys, factory, balances, venue state, quotes.
package chain

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/Debanjannnn/nearly/internal/shared/config"
	"github.com/Debanjannnn/nearly/internal/shared/near"
	"github.com/Debanjannnn/nearly/internal/shared/planner"
)

type Chain struct {
	RPC         *near.Client
	Dep         *config.Deployments
	ExecutorKey string // "ed25519:..." the factory adds to every account
}

// missing reports an RPC "this key/account does not exist" answer.
func missing(err error) bool {
	var re *near.RPCError
	if errors.As(err, &re) {
		return re.Cause.Name == "UNKNOWN_ACCESS_KEY" || re.Cause.Name == "UNKNOWN_ACCOUNT"
	}
	// view_access_key errors also arrive inside result as a plain string
	return err != nil && (strings.Contains(err.Error(), "does not exist") || strings.Contains(err.Error(), "UNKNOWN_ACCESS_KEY"))
}

// FullAccess is true when publicKey is a full-access key of account.
func (c *Chain) FullAccess(ctx context.Context, account, publicKey string) (bool, error) {
	k, err := c.RPC.ViewAccessKey(ctx, account, publicKey)
	if missing(err) {
		return false, nil
	}
	return err == nil && k.FullAccess(), err
}

// ExecutorKeyActive is true while the executor key is still on the sub-account.
func (c *Chain) ExecutorKeyActive(ctx context.Context, sub string) (bool, error) {
	_, err := c.RPC.ViewAccessKey(ctx, sub, c.ExecutorKey)
	if missing(err) {
		return false, nil
	}
	return err == nil, err
}

// FactoryAccount is owner's created sub-account, "" when none yet.
func (c *Chain) FactoryAccount(ctx context.Context, owner string) (string, error) {
	var id *string
	if err := c.RPC.View(ctx, c.Dep.Factory, "get_account", map[string]string{"owner": owner}, &id); err != nil || id == nil {
		return "", err
	}
	return *id, nil
}

func (c *Chain) RequiredDeposit(ctx context.Context) (string, error) {
	var d string
	return d, c.RPC.View(ctx, c.Dep.Factory, "required_deposit", struct{}{}, &d)
}

func (c *Chain) ftBalance(ctx context.Context, token, account string) (string, error) {
	var b string
	return b, c.RPC.View(ctx, token, "ft_balance_of", map[string]string{"account_id": account}, &b)
}

// Balances is ft_balance_of for every deployments token, keyed by symbol.
func (c *Chain) Balances(ctx context.Context, account string) (map[string]string, error) {
	out := map[string]string{}
	for sym, token := range c.Dep.Tokens {
		b, err := c.ftBalance(ctx, token, account)
		if err != nil {
			return nil, fmt.Errorf("%s balance: %w", sym, err)
		}
		out[sym] = b
	}
	return out, nil
}

// Quote is the Rhea DEX get_return view; satisfies planner.Quoter.
func (c *Chain) Quote(ctx context.Context, pool uint64, in, out string, amount *big.Int) (*big.Int, error) {
	var r string
	err := c.RPC.View(ctx, c.Dep.Venues.Dex, "get_return", map[string]any{
		"pool_id": pool, "token_in": in, "amount_in": amount.String(), "token_out": out,
	}, &r)
	if err != nil {
		return nil, &planner.Error{Code: "quote_unavailable", Detail: err.Error()}
	}
	v, ok := new(big.Int).SetString(r, 10)
	if !ok {
		return nil, &planner.Error{Code: "quote_unavailable", Detail: "bad get_return " + r}
	}
	return v, nil
}

// Live reads what account holds now for the tokens in steps (planner.ResumeWithdraw input).
func (c *Chain) Live(ctx context.Context, account string, steps []planner.Step) (planner.Live, error) {
	live := planner.Live{Lent: map[string]string{}, Wallet: map[string]string{}}
	lent, err := c.lent(ctx, account)
	if err != nil {
		return live, fmt.Errorf("lending: %w", err)
	}
	for _, st := range steps {
		switch st.Kind {
		case planner.KindWithdrawLend:
			if v, ok := lent[st.Token]; ok {
				live.Lent[st.Token] = v
			}
		case planner.KindUnstake, planner.KindSwap:
			if st.Kind == planner.KindSwap && st.TokenIn != c.Dep.Venues.Staking {
				continue
			}
			if live.Staked, err = c.staked(ctx, account); err != nil {
				return live, fmt.Errorf("staking: %w", err)
			}
		case planner.KindToOwner:
			if live.Wallet[st.Token], err = c.ftBalance(ctx, st.Token, account); err != nil {
				return live, fmt.Errorf("%s balance: %w", st.Token, err)
			}
		}
	}
	return live, nil
}

// lent maps token -> supplied atoms; Burrow balances carry extra_decimals that are cut here.
func (c *Chain) lent(ctx context.Context, account string) (map[string]string, error) {
	var acc *struct {
		Supplied []struct {
			TokenID string `json:"token_id"`
			Balance string `json:"balance"`
		} `json:"supplied"`
	}
	if err := c.RPC.View(ctx, c.Dep.Venues.Lending, "get_account", map[string]string{"account_id": account}, &acc); err != nil {
		return nil, err
	}
	out := map[string]string{}
	if acc == nil {
		return out, nil
	}
	for _, s := range acc.Supplied {
		var asset struct {
			Config struct {
				ExtraDecimals int64 `json:"extra_decimals"`
			} `json:"config"`
		}
		if err := c.RPC.View(ctx, c.Dep.Venues.Lending, "get_asset", map[string]string{"token_id": s.TokenID}, &asset); err != nil {
			return nil, err
		}
		b, ok := new(big.Int).SetString(s.Balance, 10)
		if !ok {
			return nil, fmt.Errorf("bad balance %q", s.Balance)
		}
		out[s.TokenID] = b.Quo(b, new(big.Int).Exp(big.NewInt(10), big.NewInt(asset.Config.ExtraDecimals), nil)).String()
	}
	return out, nil
}

// staked is the LiNEAR staked balance in yocto (same atoms as wNEAR).
func (c *Chain) staked(ctx context.Context, account string) (string, error) {
	var d struct {
		StakedBalance string `json:"staked_balance"`
	}
	return d.StakedBalance, c.RPC.View(ctx, c.Dep.Venues.Staking, "get_account_details", map[string]string{"account_id": account}, &d)
}
