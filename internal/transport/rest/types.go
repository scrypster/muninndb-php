package rest

import (
	"context"
	"io"
	"time"

	"github.com/scrypster/muninndb/internal/cognitive"
	"github.com/scrypster/muninndb/internal/engine/trigger"
	"github.com/scrypster/muninndb/internal/engine/vaultjob"
	"github.com/scrypster/muninndb/internal/storage"
	mbp "github.com/scrypster/muninndb/internal/transport/mbp"
)

// Re-export MBP types for convenience
type HelloRequest = mbp.HelloRequest
type HelloResponse = mbp.HelloResponse
type WriteRequest = mbp.WriteRequest
type WriteResponse = mbp.WriteResponse
type ReadRequest = mbp.ReadRequest
type ReadResponse = mbp.ReadResponse
type ActivateRequest = mbp.ActivateRequest
type ActivateResponse = mbp.ActivateResponse
type ActivationItem = mbp.ActivationItem
// LinkRequest is the REST-specific link request with proper JSON tags.
// The mbp.LinkRequest only has msgpack tags which don't decode from JSON.
type LinkRequest struct {
	SourceID string  `json:"source_id"`
	TargetID string  `json:"target_id"`
	RelType  uint16  `json:"rel_type"`
	Weight   float32 `json:"weight,omitempty"`
	Vault    string  `json:"vault,omitempty"`
}
type LinkResponse = mbp.LinkResponse
type ForgetRequest = mbp.ForgetRequest
type ForgetResponse = mbp.ForgetResponse
type StatRequest = mbp.StatRequest
type StatResponse = mbp.StatResponse
type ErrorCode = mbp.ErrorCode

const (
	ErrOK                   = mbp.ErrOK
	ErrEngramNotFound       = mbp.ErrEngramNotFound
	ErrVaultNotFound        = mbp.ErrVaultNotFound
	ErrInvalidEngram        = mbp.ErrInvalidEngram
	ErrIdempotencyViolation = mbp.ErrIdempotencyViolation
	ErrInvalidAssociation   = mbp.ErrInvalidAssociation
	ErrSubscriptionNotFound = mbp.ErrSubscriptionNotFound
	ErrThresholdInvalid     = mbp.ErrThresholdInvalid
	ErrHopDepthExceeded     = mbp.ErrHopDepthExceeded
	ErrWeightsInvalid       = mbp.ErrWeightsInvalid
	ErrAuthFailed           = mbp.ErrAuthFailed
	ErrVaultForbidden       = mbp.ErrVaultForbidden
	ErrRateLimited          = mbp.ErrRateLimited
	ErrMaxResultsExceeded   = mbp.ErrMaxResultsExceeded
	ErrStorageError         = mbp.ErrStorageError
	ErrIndexError           = mbp.ErrIndexError
	ErrEnrichmentError      = mbp.ErrEnrichmentError
	ErrShardUnavailable     = mbp.ErrShardUnavailable
	ErrInternal             = mbp.ErrInternal
)

// EngineAPI is the interface the REST server requires from the engine.
// All methods accept a context so client disconnects can cancel in-flight operations.
type EngineAPI interface {
	Hello(ctx context.Context, req *HelloRequest) (*HelloResponse, error)
	Write(ctx context.Context, req *WriteRequest) (*WriteResponse, error)
	WriteBatch(ctx context.Context, reqs []*WriteRequest) ([]*WriteResponse, []error)
	Read(ctx context.Context, req *ReadRequest) (*ReadResponse, error)
	Activate(ctx context.Context, req *ActivateRequest) (*ActivateResponse, error)
	Link(ctx context.Context, req *mbp.LinkRequest) (*LinkResponse, error)
	Forget(ctx context.Context, req *ForgetRequest) (*ForgetResponse, error)
	Stat(ctx context.Context, req *StatRequest) (*StatResponse, error)
	ListEngrams(ctx context.Context, req *ListEngramsRequest) (*ListEngramsResponse, error)
	GetEngramLinks(ctx context.Context, req *GetEngramLinksRequest) (*GetEngramLinksResponse, error)
	ListVaults(ctx context.Context) ([]string, error)
	GetSession(ctx context.Context, req *GetSessionRequest) (*GetSessionResponse, error)
	WorkerStats() cognitive.EngineWorkerStats
	// SubscribeWithDeliver registers a push subscription with a delivery function.
	// Returns the subscription ID. The deliver func is called from a goroutine
	// on each qualifying push; it must be non-blocking.
	SubscribeWithDeliver(ctx context.Context, req *mbp.SubscribeRequest, deliver trigger.DeliverFunc) (string, error)
	Unsubscribe(ctx context.Context, subID string) error
	// ClearVault removes all engrams from the named vault, leaving the vault intact.
	ClearVault(ctx context.Context, vaultName string) error
	// DeleteVault removes the named vault and all its data permanently.
	DeleteVault(ctx context.Context, vaultName string) error
	// StartClone starts an async job to clone sourceVault into a new vault named newName.
	// Returns the job immediately (202 pattern).
	StartClone(ctx context.Context, sourceVault, newName string) (*vaultjob.Job, error)
	// StartMerge starts an async job to merge sourceVault into targetVault.
	// If deleteSource is true, the source vault is deleted after the merge completes.
	StartMerge(ctx context.Context, sourceVault, targetVault string, deleteSource bool) (*vaultjob.Job, error)
	// GetVaultJob returns the status of a vault clone/merge job by ID.
	GetVaultJob(jobID string) (*vaultjob.Job, bool)
	// ExportVault synchronously exports the named vault to w as a .muninn archive.
	ExportVault(ctx context.Context, vaultName, embedderModel string, dimension int, resetMeta bool, w io.Writer) (*storage.ExportResult, error)
	// StartImport starts an async job to import a .muninn archive into a new vault.
	StartImport(ctx context.Context, vaultName, embedderModel string, dimension int, resetMeta bool, r io.Reader) (*vaultjob.Job, error)
	// ReindexFTSVault clears and rebuilds the FTS index for the named vault using
	// the current (Porter2-stemmed) tokenizer. Sets the FTS version marker to 1
	// upon completion. Returns the number of engrams re-indexed.
	ReindexFTSVault(ctx context.Context, vaultName string) (int64, error)
	// Checkpoint creates a Pebble checkpoint (point-in-time snapshot) at destDir.
	Checkpoint(destDir string) error
}

// ── Web UI types ─────────────────────────────────────────────────────────

// EngramItem is a summary of an engram for listing.
type EngramItem struct {
	ID         string   `json:"id"`
	Concept    string   `json:"concept"`
	Content    string   `json:"content"`
	Confidence float32  `json:"confidence"`
	Tags       []string `json:"tags,omitempty"`
	Vault      string   `json:"vault"`
	CreatedAt  int64    `json:"createdAt"`
}

// ListEngramsRequest lists engrams for a vault.
type ListEngramsRequest struct {
	Vault  string `json:"vault"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

// ListEngramsResponse returns paginated engrams.
type ListEngramsResponse struct {
	Engrams []EngramItem `json:"engrams"`
	Total   int          `json:"total"`
	Limit   int          `json:"limit"`
	Offset  int          `json:"offset"`
}

// AssociationItem is a graph edge for the UI.
type AssociationItem struct {
	TargetID string  `json:"targetId"`
	RelType  uint16  `json:"relType"`
	Weight   float32 `json:"weight"`
}

// GetEngramLinksRequest requests associations for an engram.
type GetEngramLinksRequest struct {
	ID    string `json:"id"`
	Vault string `json:"vault"`
}

// GetEngramLinksResponse returns association edges.
type GetEngramLinksResponse struct {
	Links []AssociationItem `json:"links"`
}

// GetSessionRequest requests recent writes.
type GetSessionRequest struct {
	Vault  string    `json:"vault"`
	Since  time.Time `json:"since"`
	Limit  int       `json:"limit"`
	Offset int       `json:"offset"`
}

// SessionItem is a single session timeline entry.
type SessionItem struct {
	ID        string `json:"id"`
	Concept   string `json:"concept"`
	Content   string `json:"content"`
	CreatedAt int64  `json:"createdAt"`
}

// GetSessionResponse returns session timeline entries.
type GetSessionResponse struct {
	Entries []SessionItem `json:"entries"`
	Total   int           `json:"total"`
	Offset  int           `json:"offset"`
	Limit   int           `json:"limit"`
}

// ErrorResponse is the standard error format returned by REST endpoints.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains error information.
type ErrorDetail struct {
	Code      ErrorCode `json:"code"`
	Message   string    `json:"message"`
	RequestID string    `json:"request_id,omitempty"`
}

// HealthResponse is returned by the health check endpoint.
type HealthResponse struct {
	Status        string `json:"status"`
	Version       string `json:"version"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	DBWritable    bool   `json:"db_writable"`
}

// ReadyResponse is returned by the ready check endpoint.
type ReadyResponse struct {
	Status string `json:"status"`
}
