package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/Debanjannnn/nearly/internal/shared/httpx"
)

const (
	minEvery = 60
	maxEvery = 366 * 24 * 3600
)

type ScheduleInput struct {
	Kind         string          `json:"kind"` // dca | recurring_buy: basket_id; rebalance: position_id
	BasketID     *string         `json:"basket_id"`
	PositionID   *string         `json:"position_id"`
	Params       json.RawMessage `json:"params"` // dca/recurring_buy: {"amount": u128 base atoms}
	EverySeconds int64           `json:"every_seconds"`
}

// validate checks shape and that the target belongs to (or is visible to) the user.
func (s *Server) validate(ctx context.Context, me string, in *ScheduleInput) error {
	if in.EverySeconds < minEvery || in.EverySeconds > maxEvery {
		return ErrInvalidSched
	}
	if len(in.Params) == 0 {
		in.Params = json.RawMessage(`{}`)
	}
	var p struct {
		Amount string `json:"amount"`
	}
	if json.Unmarshal(in.Params, &p) != nil {
		return ErrInvalidSched
	}
	switch in.Kind {
	case "dca", "recurring_buy":
		if in.BasketID == nil || in.PositionID != nil || !isUUID(*in.BasketID) || !positiveU128(p.Amount) {
			return ErrInvalidSched
		}
		_, err := s.Store.Basket(ctx, *in.BasketID, me)
		return err
	case "rebalance":
		if in.PositionID == nil || in.BasketID != nil || !isUUID(*in.PositionID) {
			return ErrInvalidSched
		}
		pos, err := s.Store.Position(ctx, me, *in.PositionID)
		if err == nil && pos.State != "open" {
			err = ErrNotOpen
		}
		return err
	}
	return ErrInvalidSched
}

func (s *Server) schedules(w http.ResponseWriter, r *http.Request) {
	v, err := s.Store.Schedules(r.Context(), account(r))
	reply(w, "schedules", v, err)
}

func (s *Server) schedule(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r); ok {
		v, err := s.Store.Schedule(r.Context(), account(r), id)
		reply(w, "schedule", v, err)
	}
}

func (s *Server) createSchedule(w http.ResponseWriter, r *http.Request) {
	var in ScheduleInput
	if !decode(w, r, &in) {
		return
	}
	ctx, me := r.Context(), account(r)
	_, err := s.ready(r) // schedules only run through an active executor key
	if err == nil {
		err = s.validate(ctx, me, &in)
	}
	var v Schedule
	if err == nil {
		v, err = s.Store.CreateSchedule(ctx, me, in)
	}
	if err != nil {
		fail(w, "create schedule", err)
		return
	}
	httpx.JSON(w, http.StatusCreated, v)
}

// updateSchedule takes every_seconds and/or active; params and target are fixed (delete + create instead).
func (s *Server) updateSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	var in struct {
		EverySeconds *int64 `json:"every_seconds"`
		Active       *bool  `json:"active"`
	}
	if !ok || !decode(w, r, &in) {
		return
	}
	if in.EverySeconds != nil && (*in.EverySeconds < minEvery || *in.EverySeconds > maxEvery) {
		fail(w, "update schedule", ErrInvalidSched)
		return
	}
	if in.Active != nil && *in.Active {
		if _, err := s.ready(r); err != nil {
			fail(w, "update schedule", err)
			return
		}
	}
	v, err := s.Store.UpdateSchedule(r.Context(), account(r), id, in.EverySeconds, in.Active)
	reply(w, "update schedule", v, err)
}

func (s *Server) deleteSchedule(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r); ok {
		err := s.Store.DeleteSchedule(r.Context(), account(r), id)
		reply(w, "delete schedule", map[string]bool{"deleted": true}, err)
	}
}
