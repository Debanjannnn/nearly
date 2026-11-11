// Package api is the HTTP surface: auth, account, baskets, positions, executions, schedules.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"sync"

	"github.com/Debanjannnn/nearly/internal/shared/config"
	"github.com/Debanjannnn/nearly/internal/shared/executor"
	"github.com/Debanjannnn/nearly/internal/shared/httpx"
	"github.com/Debanjannnn/nearly/internal/shared/intents"
	"github.com/Debanjannnn/nearly/internal/shared/planner"
	"github.com/Debanjannnn/nearly/services/api/internal/auth"
)

// Chain is the RPC surface the api reads (chain.Chain).
type Chain interface {
	planner.Quoter
	FullAccess(ctx context.Context, account, publicKey string) (bool, error)
	ExecutorKeyActive(ctx context.Context, sub string) (bool, error)
	FactoryAccount(ctx context.Context, owner string) (string, error)
	RequiredDeposit(ctx context.Context) (string, error)
	Balances(ctx context.Context, account string) (map[string]string, error)
	Live(ctx context.Context, account string, steps []planner.Step) (planner.Live, error)
}

// Runner sends steps to the executor (executor.Client).
type Runner interface {
	Execute(ctx context.Context, account, executionID string, steps []planner.Step) ([]executor.Result, error)
}

type Server struct {
	Store       Store
	Chain       Chain
	Exec        Runner
	Sessions    *auth.Sessions
	Plan        *planner.Planner
	Dep         *config.Deployments
	ExecutorKey string
	Intents     *intents.Client
	bg          sync.WaitGroup // background executor runs; tests Wait on it
}

func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/auth", s.login)
	mux.HandleFunc("GET /api/v1/baskets", s.publicBaskets)
	mux.Handle("GET /api/v1/baskets/{id}", s.optionalMe(s.basket))
	mux.HandleFunc("GET /api/v1/intents/tokens", s.intentTokens)

	mux.Handle("GET /api/v1/me", s.me(s.user))
	mux.Handle("POST /api/v1/me/account", s.me(s.createAccount))
	mux.Handle("POST /api/v1/me/revoke", s.me(s.revoke))
	mux.Handle("GET /api/v1/me/holdings", s.me(s.holdings))
	mux.Handle("GET /api/v1/me/baskets", s.me(s.myBaskets))
	mux.Handle("POST /api/v1/me/baskets", s.me(s.createBasket))
	mux.Handle("POST /api/v1/me/baskets/{id}/deploy", s.me(s.deploy))
	mux.Handle("GET /api/v1/me/positions", s.me(s.positions))
	mux.Handle("GET /api/v1/me/positions/{id}", s.me(s.position))
	mux.Handle("POST /api/v1/me/positions/{id}/withdraw", s.me(s.withdraw))
	mux.Handle("GET /api/v1/me/executions", s.me(s.executions))
	mux.Handle("GET /api/v1/me/executions/{id}", s.me(s.execution))
	mux.Handle("GET /api/v1/me/schedules", s.me(s.schedules))
	mux.Handle("POST /api/v1/me/schedules", s.me(s.createSchedule))
	mux.Handle("GET /api/v1/me/schedules/{id}", s.me(s.schedule))
	mux.Handle("PATCH /api/v1/me/schedules/{id}", s.me(s.updateSchedule))
	mux.Handle("DELETE /api/v1/me/schedules/{id}", s.me(s.deleteSchedule))
	mux.Handle("POST /api/v1/me/intents/deposit-quote", s.me(s.depositQuote))
	mux.Handle("POST /api/v1/me/intents/withdraw-quote", s.me(s.withdrawQuote))
	mux.Handle("POST /api/v1/me/intents/submit", s.me(s.submitIntent))
	mux.Handle("GET /api/v1/me/intents/status", s.me(s.intentStatus))
}

// Wait blocks until background executor runs finish.
func (s *Server) Wait() { s.bg.Wait() }

// Err is an api error with its HTTP status and snake_case code.
type Err struct {
	Status int
	Code   string
}

func (e *Err) Error() string { return e.Code }

var (
	ErrNotFound      = &Err{http.StatusNotFound, "not_found"}
	ErrBadRequest    = &Err{http.StatusBadRequest, "bad_request"}
	ErrUnauthorized  = &Err{http.StatusUnauthorized, "unauthorized"}
	ErrSignature     = &Err{http.StatusUnauthorized, "invalid_signature"}
	ErrNonceUsed     = &Err{http.StatusUnauthorized, "nonce_used"}
	ErrNoAccount     = &Err{http.StatusConflict, "account_not_ready"}
	ErrRevoked       = &Err{http.StatusConflict, "executor_revoked"}
	ErrKeyActive     = &Err{http.StatusConflict, "executor_key_active"}
	ErrNotOpen       = &Err{http.StatusConflict, "position_not_open"}
	ErrBusy          = &Err{http.StatusConflict, "execution_running"}
	ErrRPC           = &Err{http.StatusBadGateway, "rpc_unavailable"}
	ErrNoFactory     = &Err{http.StatusServiceUnavailable, "factory_not_deployed"}
	ErrInvalidSched  = &Err{http.StatusBadRequest, "invalid_schedule"}
	ErrInvalidBasket = &Err{http.StatusBadRequest, "invalid_basket"}
	ErrIntents       = &Err{http.StatusBadGateway, "intents_unavailable"}
)

type accountKey struct{}

// account is the authenticated NEAR account id (owner wallet).
func account(r *http.Request) string { a, _ := r.Context().Value(accountKey{}).(string); return a }

func (s *Server) bearer(r *http.Request) (string, bool) {
	tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	a, err := s.Sessions.Parse(strings.TrimSpace(tok))
	return a, ok && err == nil
}

// me requires a session and makes sure the users row exists (every /me table references it).
func (s *Server) me(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a, ok := s.bearer(r)
		if !ok {
			fail(w, "auth", ErrUnauthorized)
			return
		}
		if err := s.Store.EnsureUser(r.Context(), a); err != nil {
			fail(w, "ensure user", err)
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), accountKey{}, a)))
	})
}

func (s *Server) optionalMe(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a, ok := s.bearer(r); ok {
			r = r.WithContext(context.WithValue(r.Context(), accountKey{}, a))
		}
		h(w, r)
	})
}

func fail(w http.ResponseWriter, what string, err error) {
	var ae *Err
	var pe *planner.Error
	var ie *intents.Error // only 1Click 400/404 reach here, see intentsErr
	switch {
	case errors.As(err, &ie):
		httpx.JSON(w, ie.Status, map[string]string{"error": "intents_rejected", "detail": ie.Message})
	case errors.As(err, &ae):
		httpx.Error(w, ae.Status, ae.Code)
	case errors.As(err, &pe):
		status := http.StatusUnprocessableEntity
		switch {
		case pe.Code == "quote_unavailable":
			status = http.StatusBadGateway
		case pe.Code == "bad_request", strings.HasPrefix(pe.Code, "invalid_"):
			status = http.StatusBadRequest
		}
		httpx.JSON(w, status, map[string]string{"error": pe.Code, "detail": pe.Detail})
	default:
		slog.Error(what, "err", err)
		httpx.Error(w, http.StatusInternalServerError, "internal")
	}
}

func reply[T any](w http.ResponseWriter, what string, v T, err error) {
	if err != nil {
		fail(w, what, err)
		return
	}
	httpx.JSON(w, http.StatusOK, v)
}

// decode reads a JSON body; false means a 400 was written.
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(v); err != nil {
		fail(w, "decode", ErrBadRequest)
		return false
	}
	return true
}

// pathID reads a uuid path value; false means a 400 was written.
func pathID(w http.ResponseWriter, r *http.Request) (string, bool) {
	v := r.PathValue("id")
	if !isUUID(v) {
		fail(w, "path id", ErrBadRequest)
		return "", false
	}
	return v, true
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}

// positiveU128 is true for a decimal string in 1..2^128-1.
func positiveU128(s string) bool {
	v, ok := new(big.Int).SetString(s, 10)
	return ok && v.Sign() > 0 && v.BitLen() <= 128
}

// rpc wraps a chain read error so the client sees 502.
func rpc(what string, err error) error {
	if err == nil {
		return nil
	}
	slog.Error(what, "err", err)
	return ErrRPC
}
