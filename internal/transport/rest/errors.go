package rest

import (
	"encoding/json"
	"net/http"
)

// APIError is the standard JSON error envelope used by middleware-level responses
// (rate limiting, cluster auth). Handler-level errors continue to use ErrorResponse
// via sendError — this type is used only for paths that bypass the normal chain.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeError writes a JSON error response using the APIError envelope.
// It is used by middleware that runs outside the normal handler chain
// (e.g. rate limiting, cluster auth) where sendError is not available.
func writeError(w http.ResponseWriter, httpStatus int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	_ = json.NewEncoder(w).Encode(APIError{Code: code, Message: message})
}
