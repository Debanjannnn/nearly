package api

import (
	"net/http"
	"strings"

	"github.com/Debanjannnn/nearly/internal/shared/httpx"
	"github.com/Debanjannnn/nearly/internal/shared/planner"
)

func (s *Server) createBasket(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name       string         `json:"name"`
		Definition planner.Basket `json:"definition"`
		Public     bool           `json:"public"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 80 {
		fail(w, "basket", ErrInvalidBasket)
		return
	}
	if err := s.Plan.Validate(in.Definition); err != nil {
		fail(w, "basket", err)
		return
	}
	b, err := s.Store.CreateBasket(r.Context(), account(r), in.Name, in.Definition, in.Public)
	if err != nil {
		fail(w, "create basket", err)
		return
	}
	httpx.JSON(w, http.StatusCreated, b)
}

func (s *Server) publicBaskets(w http.ResponseWriter, r *http.Request) {
	v, err := s.Store.PublicBaskets(r.Context())
	reply(w, "baskets", v, err)
}

func (s *Server) myBaskets(w http.ResponseWriter, r *http.Request) {
	v, err := s.Store.MyBaskets(r.Context(), account(r))
	reply(w, "my baskets", v, err)
}

func (s *Server) basket(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r); ok {
		v, err := s.Store.Basket(r.Context(), id, account(r))
		reply(w, "basket", v, err)
	}
}
