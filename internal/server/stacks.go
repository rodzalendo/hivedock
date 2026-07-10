package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/rogalinski/hivedock/internal/stacks"
)

func (a *api) listStacks(w http.ResponseWriter, r *http.Request) {
	list, err := a.stacks.List(r.Context())
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
	name := chi.URLParam(r, "name")
	st, ok, err := a.stacks.Get(r.Context(), name)
	if err != nil {
		a.logger.Error("get stack", "name", name, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to get stack: "+err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "stack not found: "+name)
		return
	}
	writeJSON(w, http.StatusOK, st)
}
