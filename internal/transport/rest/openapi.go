package rest

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.yaml
var openapiSpec []byte

// handleOpenAPISpec serves the embedded OpenAPI 3.0 specification as YAML.
// GET /api/openapi.yaml
func (s *Server) handleOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	w.Write(openapiSpec) //nolint:errcheck
}
