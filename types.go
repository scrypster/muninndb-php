package muninn

import (
	"errors"
	"time"
)

// ErrNotFound is returned when an engram with the requested ID does not exist.
var ErrNotFound = errors.New("engram not found")

// Engram is a single memory record returned by the public API.
type Engram struct {
	ID         string    `json:"id"`
	Concept    string    `json:"concept"`
	Content    string    `json:"content"`
	Summary    string    `json:"summary,omitempty"`
	State      string    `json:"state,omitempty"`
	Score      float64   `json:"score,omitempty"`
	Confidence float32   `json:"confidence,omitempty"`
	Tags       []string  `json:"tags,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	LastAccess time.Time `json:"last_access"`
}
