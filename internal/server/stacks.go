package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/rogalinski/hivedock/internal/stacks"
)

func (a *api) listStacks(w http.ResponseWriter, r *http.Request) {
	be, err := a.backendFor(hostParam(r))
	if err != nil {
		a.httpError(w, err)
		return
	}
	list, err := be.ListStacks(r.Context())
	if err != nil {
		a.logger.Error("list stacks", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list stacks: "+err.Error())
		return
	}
	if list == nil {
		list = []stacks.Stack{} // empty dir is a valid empty state, not null
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *api) getStack(w http.ResponseWriter, r *http.Request) {
	be, err := a.backendFor(hostParam(r))
	if err != nil {
		a.httpError(w, err)
		return
	}
	st, err := be.GetStack(r.Context(), chi.URLParam(r, "name"))
	if err != nil {
		a.httpError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}
