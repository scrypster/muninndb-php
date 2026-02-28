package rest

import (
	"net/http"
	"time"
)

// handleObservability returns the full system observability snapshot.
// GET /api/admin/observability
func (s *Server) handleObservability(w http.ResponseWriter, r *http.Request) {
	uptimeSeconds := int64(time.Since(s.startTime).Seconds())
	version := s.version
	if version == "" {
		version = "dev"
	}

	snap, err := s.engine.Observability(r.Context(), version, uptimeSeconds)
	if err != nil {
		s.sendError(r, w, http.StatusInternalServerError, ErrInternal, "observability: "+err.Error())
		return
	}
	s.sendJSON(w, http.StatusOK, snap)
}
