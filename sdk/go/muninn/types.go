package muninn

// Engram represents a single memory unit in MuninnDB.
type Engram struct {
	ID          string   `json:"id"`
	Concept     string   `json:"concept"`
	Content     string   `json:"content"`
	Confidence  float64  `json:"confidence"`
	Relevance   float64  `json:"relevance"`
	Stability   float64  `json:"stability"`
	AccessCount int      `json:"access_count"`
	Tags        []string `json:"tags"`
	State       int      `json:"state"`
	CreatedAt   int64    `json:"created_at"`
	UpdatedAt   int64    `json:"updated_at"`
	LastAccess  *int64   `json:"last_access,omitempty"`
	MemoryType  int      `json:"memory_type,omitempty"`
	TypeLabel   string   `json:"type_label,omitempty"`
}

// InlineEntity is a caller-provided entity for inline enrichment.
type InlineEntity struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// InlineRelationship is a caller-provided relationship for inline enrichment.
type InlineRelationship struct {
	TargetID string  `json:"target_id"`
	Relation string  `json:"relation"`
	Weight   float64 `json:"weight,omitempty"`
}

// WriteRequest represents a request to write an engram.
type WriteRequest struct {
	Vault         string                 `json:"vault"`
	Concept       string                 `json:"concept"`
	Content       string                 `json:"content"`
	Tags          []string               `json:"tags,omitempty"`
	Confidence    float64                `json:"confidence,omitempty"`
	Stability     float64                `json:"stability,omitempty"`
	Embedding     []float64              `json:"embedding,omitempty"`
	Associations  map[string]interface{} `json:"associations,omitempty"`
	MemoryType    *int                   `json:"memory_type,omitempty"`
	TypeLabel     string                 `json:"type_label,omitempty"`
	Summary       string                 `json:"summary,omitempty"`
	Entities      []InlineEntity         `json:"entities,omitempty"`
	Relationships []InlineRelationship   `json:"relationships,omitempty"`
}

// WriteResponse represents a response from writing an engram.
type WriteResponse struct {
	ID        string `json:"id"`
	CreatedAt int64  `json:"created_at"`
	Hint      string `json:"hint,omitempty"`
}

// BatchWriteResult holds the result for a single item in a batch write.
type BatchWriteResult struct {
	Index  int    `json:"index"`
	ID     string `json:"id,omitempty"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// BatchWriteResponse holds the response from a batch write operation.
type BatchWriteResponse struct {
	Results []BatchWriteResult `json:"results"`
}

// ActivateRequest represents a request to activate memory.
type ActivateRequest struct {
	Vault     string   `json:"vault"`
	Context   []string `json:"context"`
	MaxResults int     `json:"max_results,omitempty"`
	Threshold float64  `json:"threshold,omitempty"`
	MaxHops   int      `json:"max_hops,omitempty"`
	IncludeWhy bool    `json:"include_why,omitempty"`
	BriefMode string   `json:"brief_mode,omitempty"`
}

// ActivationItem represents a single activated memory item.
type ActivationItem struct {
	ID         string   `json:"id"`
	Concept    string   `json:"concept"`
	Content    string   `json:"content"`
	Score      float64  `json:"score"`
	Confidence float64  `json:"confidence"`
	Why        *string  `json:"why,omitempty"`
	HopPath    []string `json:"hop_path,omitempty"`
	Dormant    bool     `json:"dormant,omitempty"`
	MemoryType int      `json:"memory_type,omitempty"`
	TypeLabel  string   `json:"type_label,omitempty"`
}

// BriefSentence represents a sentence extracted by brief mode.
type BriefSentence struct {
	EngramID string  `json:"engram_id"`
	Text     string  `json:"text"`
	Score    float64 `json:"score"`
}

// ActivateResponse represents a response from activating memory.
type ActivateResponse struct {
	QueryID    string             `json:"query_id"`
	TotalFound int                `json:"total_found"`
	Activations []ActivationItem  `json:"activations"`
	LatencyMs  float64            `json:"latency_ms,omitempty"`
	Brief      []BriefSentence    `json:"brief,omitempty"`
}

// CoherenceResult contains coherence metrics for a vault.
type CoherenceResult struct {
	Score                  float64 `json:"score"`
	OrphanRatio            float64 `json:"orphan_ratio"`
	ContradictionDensity   float64 `json:"contradiction_density"`
	DuplicationPressure    float64 `json:"duplication_pressure"`
	TemporalVariance       float64 `json:"temporal_variance"`
	TotalEngrams           int     `json:"total_engrams"`
}

// StatsResponse represents the response from the stats endpoint.
type StatsResponse struct {
	EngramCount int                          `json:"engram_count"`
	VaultCount  int                          `json:"vault_count"`
	StorageBytes int                         `json:"storage_bytes"`
	Coherence   map[string]CoherenceResult  `json:"coherence,omitempty"`
}

// LinkRequest represents a request to link two engrams.
type LinkRequest struct {
	Vault    string  `json:"vault"`
	SourceID string  `json:"source_id"`
	TargetID string  `json:"target_id"`
	RelType  int     `json:"rel_type"`
	Weight   float64 `json:"weight"`
}

// ForgetRequest represents a request to forget an engram.
type ForgetRequest struct {
	ID    string `json:"id"`
	Vault string `json:"vault"`
}

// Push represents an SSE push event from subscription.
type Push struct {
	SubscriptionID string `json:"subscription_id"`
	Trigger        string `json:"trigger"`
	PushNumber     int    `json:"push_number"`
	EngramID       *string `json:"engram_id,omitempty"`
	At             *int64  `json:"at,omitempty"`
}

// HealthResponse represents the response from the health endpoint.
type HealthResponse struct {
	Status string `json:"status"`
}

// ErrorResponse represents an error response from the API.
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}
