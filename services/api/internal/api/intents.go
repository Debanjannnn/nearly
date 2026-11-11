package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Debanjannnn/nearly/internal/shared/intents"
)

// quoteTTL is how long a live 1Click deposit address accepts funds before it refunds.
const quoteTTL = time.Hour

type intentQuoteIn struct {
	OriginAsset      string `json:"origin_asset"`
	DestinationAsset string `json:"destination_asset"` // withdraw only
	Amount           string `json:"amount"`
	SwapType         string `json:"swap_type"`    // default EXACT_INPUT
	SlippageBps      int    `json:"slippage_bps"` // default 100
	RefundTo         string `json:"refund_to"`    // deposit: origin chain address
	Recipient        string `json:"recipient"`    // withdraw: destination chain address
	Dry              bool   `json:"dry"`
}

func (in *intentQuoteIn) request(origin, dest, recipient, refundTo string) (intents.QuoteRequest, bool) {
	if in.SwapType == "" {
		in.SwapType = "EXACT_INPUT"
	}
	if in.SlippageBps == 0 {
		in.SlippageBps = 100
	}
	ok := origin != "" && dest != "" && recipient != "" && refundTo != "" && positiveU128(in.Amount) &&
		in.SlippageBps > 0 && in.SlippageBps <= 1000 &&
		(in.SwapType == "EXACT_INPUT" || in.SwapType == "EXACT_OUTPUT" || in.SwapType == "FLEX_INPUT")
	return intents.QuoteRequest{
		Dry: in.Dry, SwapType: in.SwapType, SlippageTolerance: in.SlippageBps,
		OriginAsset: origin, DepositType: "ORIGIN_CHAIN", DestinationAsset: dest, Amount: in.Amount,
		RefundTo: refundTo, RefundType: "ORIGIN_CHAIN", Recipient: recipient, RecipientType: "DESTINATION_CHAIN",
		Deadline: time.Now().Add(quoteTTL).UTC().Truncate(time.Second),
	}, ok
}

func (s *Server) intentTokens(w http.ResponseWriter, r *http.Request) {
	// shortcut: proxied per request, cache if 1Click rate limits us
	toks, err := s.Intents.Tokens(r.Context())
	reply(w, "intents tokens", toks, intentsErr(err))
}

// depositQuote swaps any asset from another chain into USDC on the user's sub-account; refunds go back to origin.
func (s *Server) depositQuote(w http.ResponseWriter, r *http.Request) {
	var in intentQuoteIn
	if !decode(w, r, &in) {
		return
	}
	u, err := s.Store.User(r.Context(), account(r))
	if err != nil {
		fail(w, "user", err)
		return
	}
	if u.SubAccount == nil {
		fail(w, "deposit quote", ErrNoAccount)
		return
	}
	q, ok := in.request(in.OriginAsset, "nep141:"+s.Dep.Tokens["usdc"], *u.SubAccount, in.RefundTo)
	s.quote(w, r, "deposit", q, ok)
}

// withdrawQuote routes a NEAR asset the owner holds to an address on another chain; refunds go back to the owner.
func (s *Server) withdrawQuote(w http.ResponseWriter, r *http.Request) {
	var in intentQuoteIn
	if !decode(w, r, &in) {
		return
	}
	q, ok := in.request(in.OriginAsset, in.DestinationAsset, in.Recipient, account(r))
	s.quote(w, r, "withdraw", q, ok && strings.HasPrefix(in.OriginAsset, "nep141:"))
}

// quote asks 1Click and records live quotes so status survives restarts.
func (s *Server) quote(w http.ResponseWriter, r *http.Request, kind string, q intents.QuoteRequest, ok bool) {
	if !ok {
		fail(w, kind+" quote", ErrBadRequest)
		return
	}
	res, err := s.Intents.Quote(r.Context(), q)
	if err != nil {
		fail(w, kind+" quote", intentsErr(err))
		return
	}
	if res.Quote.DepositAddress != "" {
		err = s.Store.AddIntent(r.Context(), account(r), kind, res.Quote.DepositAddress, res.Quote.DepositMemo, res.Raw)
	}
	reply(w, kind+" quote", res, err)
}

// intentStatus lists the user's swaps (or one, by ?deposit_address), refreshing non-terminal ones from 1Click.
func (s *Server) intentStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.Store.Intents(ctx, account(r), r.URL.Query().Get("deposit_address"))
	if err != nil {
		fail(w, "intents", err)
		return
	}
	for i := range list {
		x := &list[i]
		if intents.Terminal(x.Status) {
			continue
		}
		st, err := s.Intents.Status(ctx, x.DepositAddress, x.DepositMemo)
		var ie *intents.Error
		if errors.As(err, &ie) && ie.Status == http.StatusNotFound {
			continue // 1Click has not seen the address yet
		}
		if err != nil {
			slog.Warn("intents status", "addr", x.DepositAddress, "err", err)
			continue // serve the stored status
		}
		if err := s.Store.SetIntentStatus(ctx, x, st.Status, "", st.SwapDetails); err != nil {
			fail(w, "intent status", err)
			return
		}
	}
	reply(w, "intents", list, nil)
}

// submitIntent passes the user's deposit tx hash to 1Click so it is picked up sooner.
func (s *Server) submitIntent(w http.ResponseWriter, r *http.Request) {
	var in struct {
		DepositAddress string `json:"deposit_address"`
		DepositMemo    string `json:"deposit_memo"`
		TxHash         string `json:"tx_hash"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.TxHash == "" || len(in.TxHash) > 128 {
		fail(w, "submit intent", ErrBadRequest)
		return
	}
	ctx, me := r.Context(), account(r)
	x, err := s.Store.Intent(ctx, me, in.DepositAddress, in.DepositMemo)
	if err != nil {
		fail(w, "intent", err)
		return
	}
	sub := intents.SubmitRequest{TxHash: in.TxHash, DepositAddress: x.DepositAddress, Memo: x.DepositMemo}
	if x.Kind == "withdraw" {
		sub.NearSenderAccount = me // owner sends the NEAR asset
	}
	st, err := s.Intents.Submit(ctx, sub)
	if err != nil {
		fail(w, "submit intent", intentsErr(err))
		return
	}
	err = s.Store.SetIntentStatus(ctx, &x, st.Status, in.TxHash, st.SwapDetails)
	reply(w, "submit intent", x, err)
}

// intentsErr maps 1Click errors: its 400s are the caller's input, anything else is upstream trouble.
func intentsErr(err error) error {
	var ie *intents.Error
	switch {
	case err == nil:
		return nil
	case errors.As(err, &ie) && (ie.Status == http.StatusBadRequest || ie.Status == http.StatusNotFound):
		return ie
	default:
		slog.Error("1click", "err", err)
		return ErrIntents
	}
}
