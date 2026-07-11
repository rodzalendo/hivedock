package server

import (
	"context"
	"net/http"
	"time"
)

// prune removes dangling images and stale build cache via the Docker client.
// Conservative by design: tagged images, containers, volumes, and networks are
// never touched, so it is always safe to run after image updates.
func (a *api) prune(w http.ResponseWriter, r *http.Request) {
	if a.docker == nil {
		writeError(w, http.StatusServiceUnavailable, "docker daemon unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	rep, err := a.docker.Prune(ctx)
	if err != nil {
		a.logger.Error("prune", "err", err)
		writeError(w, http.StatusInternalServerError, "prune failed: "+err.Error())
		return
	}
	a.logger.Info("prune complete", "imagesDeleted", rep.ImagesDeleted, "spaceReclaimed", rep.SpaceReclaimed)
	writeJSON(w, http.StatusOK, rep)
}
