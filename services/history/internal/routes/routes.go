// Package routes serves the stored daily series and the backtest endpoint.
package routes

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/Debanjannnn/nearly/internal/shared/httpx"
	"github.com/Debanjannnn/nearly/internal/shared/planner"
	"github.com/Debanjannnn/nearly/services/history/internal/backtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const day = backtest.Day

type Server struct {
	DB     *pgxpool.Pool
	Tokens map[string]string // deployments tokens, for basket validation
	Now    func() time.Time
}

func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/series/sources", s.sources)
	mux.HandleFunc("GET /api/v1/series/rates", s.rates)
	mux.HandleFunc("GET /api/v1/series/prices", s.prices)
	mux.HandleFunc("POST /api/v1/backtest", s.backtest)
}

func fail(w http.ResponseWriter, status int, code, detail string) {
	httpx.JSON(w, status, map[string]string{"error": code, "detail": detail})
}

func internal(w http.ResponseWriter, what string, err error) {
	slog.Error(what, "err", err)
	httpx.Error(w, http.StatusInternalServerError, "internal")
}

// window parses from/to (YYYY-MM-DD); defaults to the year ending today.
func (s *Server) window(fromS, toS string) (from, to time.Time, err error) {
	to = s.Now().UTC().Truncate(24 * time.Hour)
	if toS != "" {
		if to, err = time.Parse(day, toS); err != nil {
			return
		}
	}
	from = to.AddDate(-1, 0, 0)
	if fromS != "" {
		if from, err = time.Parse(day, fromS); err != nil {
			return
		}
	}
	if from.After(to) || to.Sub(from) > 3*366*24*time.Hour {
		err = errors.New("from must be <= to and span <= 3y")
	}
	return
}

// rows returns each row as column -> value; numerics are cast to text in SQL so they stay exact.
func (s *Server) rows(ctx context.Context, q string, args ...any) ([]map[string]any, error) {
	rs, err := s.DB.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rs, pgx.RowToMap)
	if out == nil {
		out = []map[string]any{}
	}
	return out, err
}

// sources lists every series the backtest can use, so the basket builder knows what is available.
func (s *Server) sources(w http.ResponseWriter, r *http.Request) {
	data, err := s.rows(r.Context(), `
		select source, token, to_char(min(day),'YYYY-MM-DD') as first, to_char(max(day),'YYYY-MM-DD') as last, count(*) as days
		from series_rate group by 1, 2
		union all
		select 'price', token, to_char(min(day),'YYYY-MM-DD'), to_char(max(day),'YYYY-MM-DD'), count(*) from series_price group by 2
		order by 1, 2`)
	if err != nil {
		internal(w, "sources", err)
		return
	}
	httpx.JSON(w, 200, map[string]any{"data": data})
}

func (s *Server) rates(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, to, err := s.window(q.Get("from"), q.Get("to"))
	if err != nil {
		fail(w, 400, "invalid_range", err.Error())
		return
	}
	data, err := s.rows(r.Context(), `select source, token, to_char(day,'YYYY-MM-DD') as day, apy::text as apy
		from series_rate where day between $1 and $2 and ($3 = '' or source = $3) and ($4 = '' or token = $4)
		order by source, token, day`, from, to, q.Get("source"), q.Get("token"))
	if err != nil {
		internal(w, "rates", err)
		return
	}
	httpx.JSON(w, 200, map[string]any{"data": data})
}

func (s *Server) prices(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	tok := q.Get("token")
	if tok == "" {
		fail(w, 400, "invalid_token", "token is required, e.g. wnear")
		return
	}
	from, to, err := s.window(q.Get("from"), q.Get("to"))
	if err != nil {
		fail(w, 400, "invalid_range", err.Error())
		return
	}
	data, err := s.rows(r.Context(), `select to_char(day,'YYYY-MM-DD') as day, usd::text as usd
		from series_price where token = $1 and day between $2 and $3 order by day`, tok, from, to)
	if err != nil {
		internal(w, "prices", err)
		return
	}
	httpx.JSON(w, 200, map[string]any{"token": tok, "data": data})
}

// BacktestRequest: amount is USD as a decimal string; basket is the shared planner basket.
type BacktestRequest struct {
	Basket  planner.Basket `json:"basket"`
	Amount  string         `json:"amount"`
	From    string         `json:"from"`
	To      string         `json:"to"`
	SwapBps *float64       `json:"swap_bps"` // default 30: Rhea pool fee
}

func (s *Server) backtest(w http.ResponseWriter, r *http.Request) {
	var req BacktestRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		fail(w, 400, "bad_request", err.Error())
		return
	}
	if err := (&planner.Planner{Tokens: s.Tokens}).Validate(req.Basket); err != nil {
		var pe *planner.Error
		if errors.As(err, &pe) {
			fail(w, 400, pe.Code, pe.Detail)
			return
		}
		internal(w, "validate", err)
		return
	}
	amount, err := strconv.ParseFloat(req.Amount, 64)
	if err != nil {
		fail(w, 400, "bad_request", "amount must be a decimal string")
		return
	}
	in := backtest.Input{Basket: req.Basket, Amount: amount, SwapBps: 30}
	if req.SwapBps != nil {
		in.SwapBps = *req.SwapBps
	}
	if in.From, in.To, err = s.window(req.From, req.To); err != nil {
		fail(w, 400, "invalid_range", err.Error())
		return
	}
	series, err := s.load(r.Context(), in.From, in.To)
	if err != nil {
		internal(w, "load series", err)
		return
	}
	res, err := backtest.Run(in, series)
	var be *backtest.Error
	if errors.As(err, &be) {
		fail(w, 422, be.Code, be.Msg)
		return
	}
	if err != nil {
		internal(w, "backtest", err)
		return
	}
	httpx.JSON(w, 200, res)
}

// load reads every series in [from-30d, to]; the extra month seeds forward-fill at the window start.
func (s *Server) load(ctx context.Context, from, to time.Time) (backtest.Series, error) {
	out := backtest.Series{Price: map[string]map[string]float64{}, APY: map[string]map[string]float64{}}
	put := func(m map[string]map[string]float64, k, d string, v float64) {
		if m[k] == nil {
			m[k] = map[string]float64{}
		}
		m[k][d] = v
	}
	var k, d string
	var v float64
	lo := from.AddDate(0, 0, -30)
	rows, err := s.DB.Query(ctx, `select source || ':' || token, to_char(day,'YYYY-MM-DD'), apy::float8
		from series_rate where day between $1 and $2`, lo, to)
	if err != nil {
		return out, err
	}
	if _, err := pgx.ForEachRow(rows, []any{&k, &d, &v}, func() error { put(out.APY, k, d, v); return nil }); err != nil {
		return out, err
	}
	rows, err = s.DB.Query(ctx, `select token, to_char(day,'YYYY-MM-DD'), usd::float8
		from series_price where day between $1 and $2`, lo, to)
	if err != nil {
		return out, err
	}
	_, err = pgx.ForEachRow(rows, []any{&k, &d, &v}, func() error { put(out.Price, k, d, v); return nil })
	return out, err
}
