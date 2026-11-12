package api

import (
	"context"
	"encoding/base64"
	"log/slog"
	"net/http"

	"github.com/Debanjannnn/nearly/internal/shared/httpx"
	"github.com/Debanjannnn/nearly/services/api/internal/auth"
)

// nonce issues a single-use login challenge the client must sign within the ttl.
func (s *Server) nonce(w http.ResponseWriter, r *http.Request) {
	n := auth.NewNonce()
	err := s.Store.IssueNonce(r.Context(), n)
	reply(w, "issue nonce", map[string]string{"nonce": base64.StdEncoding.EncodeToString(n)}, err)
}

// login checks a NEP-413 signed "nearly login" from a full-access key of account_id, then issues a session.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AccountID string `json:"account_id"`
		PublicKey string `json:"public_key"`
		Signature string `json:"signature"`
		Nonce     string `json:"nonce"` // base64, from GET /auth/nonce
	}
	if !decode(w, r, &in) {
		return
	}
	nonce, err := base64.StdEncoding.DecodeString(in.Nonce)
	if err != nil || in.AccountID == "" || len(in.AccountID) > 64 {
		fail(w, "login", ErrBadRequest)
		return
	}
	if auth.Verify(in.PublicKey, in.Signature, nonce) != nil {
		fail(w, "login", ErrSignature)
		return
	}
	ctx := r.Context()
	full, err := s.Chain.FullAccess(ctx, in.AccountID, in.PublicKey)
	if err != nil {
		fail(w, "login", rpc("view_access_key", err))
		return
	}
	if !full {
		fail(w, "login", ErrSignature)
		return
	}
	if err := s.Store.UseNonce(ctx, nonce, in.AccountID); err != nil {
		fail(w, "login nonce", err)
		return
	}
	if err := s.Store.EnsureUser(ctx, in.AccountID); err != nil {
		fail(w, "ensure user", err)
		return
	}
	tok, err := s.Sessions.Issue(in.AccountID)
	reply(w, "issue", map[string]string{"token": tok}, err)
}

func (s *Server) user(w http.ResponseWriter, r *http.Request) {
	u, err := s.Store.User(r.Context(), account(r))
	reply(w, "user", u, err)
}

// createAccount returns the user's sub-account once the factory made it. Until then it returns the
// create_account tx for the user to sign with NEAR Connect: the api holds no funded relayer key.
func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) {
	ctx, me := r.Context(), account(r)
	u, err := s.Store.User(ctx, me)
	if err != nil {
		fail(w, "user", err)
		return
	}
	if u.SubAccount != nil {
		httpx.JSON(w, http.StatusOK, map[string]string{"account_id": *u.SubAccount, "key_status": u.KeyStatus})
		return
	}
	if s.Dep.Factory == "" {
		fail(w, "account", ErrNoFactory)
		return
	}
	sub, err := s.Chain.FactoryAccount(ctx, me)
	if err != nil {
		fail(w, "account", rpc("factory get_account", err))
		return
	}
	if sub != "" {
		on, err := s.Chain.ExecutorKeyActive(ctx, sub)
		if err != nil {
			fail(w, "account", rpc("executor key", err))
			return
		}
		if !on {
			// factory maps the name before the create batch lands, and unmaps it if the batch fails
			httpx.JSON(w, http.StatusAccepted, map[string]any{"account_id": nil, "key_status": "pending"})
			return
		}
		if err := s.Store.SetAccount(ctx, me, sub, "active"); err != nil {
			fail(w, "set account", err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]string{"account_id": sub, "key_status": "active"})
		return
	}
	deposit, err := s.Chain.RequiredDeposit(ctx)
	if err != nil {
		fail(w, "account", rpc("required_deposit", err))
		return
	}
	if err := s.Store.SetAccount(ctx, me, "", "pending"); err != nil {
		fail(w, "set account", err)
		return
	}
	// near-connect signAndSendTransaction shape; client calls this route again once the tx lands
	httpx.JSON(w, http.StatusAccepted, map[string]any{
		"account_id": nil,
		"key_status": "pending",
		"tx": map[string]any{
			"receiverId": s.Dep.Factory,
			"actions": []any{map[string]any{"type": "FunctionCall", "params": map[string]any{
				"methodName": "create_account",
				"args":       map[string]string{"owner": me, "executor_pubkey": s.ExecutorKey},
				"gas":        "300000000000000", // factory loads 0.5MB wasm + 110 Tgas inner; 150 ran out
				"deposit":    deposit,
			}}},
		},
	})
}

// revoke records that the owner removed the executor key on chain and deactivates every schedule.
func (s *Server) revoke(w http.ResponseWriter, r *http.Request) {
	ctx, me := r.Context(), account(r)
	u, err := s.Store.User(ctx, me)
	if err == nil && u.SubAccount == nil {
		err = ErrNoAccount
	}
	if err != nil {
		fail(w, "revoke", err)
		return
	}
	on, err := s.Chain.ExecutorKeyActive(ctx, *u.SubAccount)
	if err != nil {
		fail(w, "revoke", rpc("executor key", err))
		return
	}
	if on {
		fail(w, "revoke", ErrKeyActive) // owner must sign the on-chain revoke first
		return
	}
	n, err := s.Store.Revoke(ctx, me)
	reply(w, "revoke", map[string]any{"key_status": "revoked", "schedules_deactivated": n}, err)
}

// keyGone does what /me/revoke does once a run hit a missing executor key, so schedules stop
// even if the client never called it.
func (s *Server) keyGone(ctx context.Context, me, sub string) {
	on, err := s.Chain.ExecutorKeyActive(ctx, sub)
	if err != nil || on {
		slog.Warn("auto revoke skipped", "account", me, "key_on", on, "err", err)
		return
	}
	n, err := s.Store.Revoke(ctx, me)
	if err != nil {
		slog.Error("auto revoke", "account", me, "err", err)
		return
	}
	slog.Info("executor key gone on chain, revoked", "account", me, "schedules_deactivated", n)
}

func (s *Server) holdings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u, err := s.Store.User(ctx, account(r))
	if err == nil && u.SubAccount == nil {
		err = ErrNoAccount
	}
	if err != nil {
		fail(w, "holdings", err)
		return
	}
	bals, err := s.Chain.Balances(ctx, *u.SubAccount)
	if err != nil {
		fail(w, "holdings", rpc("ft_balance_of", err))
		return
	}
	type holding struct {
		Token   string `json:"token"`
		Balance string `json:"balance"`
	}
	out := map[string]holding{}
	for sym, b := range bals {
		out[sym] = holding{s.Dep.Tokens[sym], b}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"account_id": *u.SubAccount, "tokens": out})
}

// ready returns the sub-account of a user whose executor key is active.
func (s *Server) ready(r *http.Request) (string, error) {
	u, err := s.Store.User(r.Context(), account(r))
	switch {
	case err != nil:
		return "", err
	case u.SubAccount == nil:
		return "", ErrNoAccount
	case u.KeyStatus != "active":
		return "", ErrRevoked
	}
	return *u.SubAccount, nil
}
