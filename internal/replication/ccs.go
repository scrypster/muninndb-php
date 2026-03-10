package replication

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/scrypster/muninndb/internal/transport/mbp"
)

// HebbianSampler is the interface used by CCSProbe to sample and read
// Hebbian association weights. It is kept separate from HebbianStore to
// avoid adding sampling concerns to the core storage interface.
type HebbianSampler interface {
	// SampleKeys returns up to n random association weight keys from the store.
	SampleKeys(n int) ([][16]byte, error)
	// GetAssocWeightsForKeys returns the current weight for each key.
	// Keys not found in the store are omitted from the result.
	GetAssocWeightsForKeys(keys [][16]byte) (map[[16]byte]float64, error)
}

// CCSResult holds the result of a single CCS measurement round.
type CCSResult struct {
	Score      float64            `json:"score"`       // 0.0–1.0
	Assessment string             `json:"assessment"`  // "excellent","good","degraded","critical"
	NodeScores map[string]float64 `json:"node_scores"` // per-lobe hash match result (1.0 or 0.0)
	SampledAt  time.Time          `json:"sampled_at"`
}

// ccsAssessment maps a score to a human-readable label.
func ccsAssessment(score float64) string {
	switch {
	case score > 0.99:
		return "excellent"
	case score > 0.95:
		return "good"
	case score > 0.90:
		return "degraded"
	default:
		return "critical"
	}
}

// CCSProbe computes the Cognitive Consistency Score by sampling Hebbian weights
// on the Cortex, broadcasting the sampled keys to all Lobes, collecting their
// hashes, and computing the fraction of Lobes with matching cognitive state.
type CCSProbe struct {
	store   HebbianSampler
	coord   *ClusterCoordinator
	mu      sync.RWMutex
	last    CCSResult
	sampleN int // number of keys to sample per round (default 100)

	// probeIntervalS is the CCS probe interval in seconds, hot-reloadable via SetInterval.
	probeIntervalS atomic.Int32

	// pending tracks in-flight probe rounds: requestID -> channel of responses.
	pendingMu sync.Mutex
	pending   map[string]chan mbp.CCSResponseMsg
}

// NewCCSProbe creates a CCSProbe. store may be nil; if nil the probe returns
// score=1.0 (no cognitive state to compare).
// defaultCCSIntervalS is the default CCS probe interval in seconds.
const defaultCCSIntervalS = 30

func NewCCSProbe(store HebbianSampler, coord *ClusterCoordinator) *CCSProbe {
	p := &CCSProbe{
		store:   store,
		coord:   coord,
		sampleN: 100,
		pending: make(map[string]chan mbp.CCSResponseMsg),
		last: CCSResult{
			Score:      1.0,
			Assessment: "excellent",
			NodeScores: map[string]float64{},
			SampledAt:  time.Now(),
		},
	}
	p.probeIntervalS.Store(defaultCCSIntervalS)
	return p
}

// LastResult returns the most recently computed CCSResult (thread-safe).
func (p *CCSProbe) LastResult() CCSResult {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.last
}

// SetInterval updates the CCS probe interval. Safe to call after Run().
// The change takes effect on the next ticker reset.
func (p *CCSProbe) SetInterval(d time.Duration) {
	s := int32(d.Seconds())
	if s < 1 {
		s = 1
	}
	p.probeIntervalS.Store(s)
}

// Run starts the periodic CCS sampling loop. It runs until ctx is cancelled.
// Only the Cortex node performs probes; Lobes are passive responders.
// The probe interval can be updated at runtime via SetInterval.
func (p *CCSProbe) Run(ctx context.Context) {
	interval := time.Duration(p.probeIntervalS.Load()) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		// Reset ticker if interval has changed.
		newInterval := time.Duration(p.probeIntervalS.Load()) * time.Second
		if newInterval != interval {
			interval = newInterval
			ticker.Reset(interval)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if p.coord.IsLeader() {
				p.probe(ctx)
			}
		}
	}
}

// probe executes one CCS measurement round.
func (p *CCSProbe) probe(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	lobes := p.coord.joinHandler.Members()
	if len(lobes) == 0 {
		// Single-node cluster: always consistent.
		p.mu.Lock()
		p.last = CCSResult{
			Score:      1.0,
			Assessment: "excellent",
			NodeScores: map[string]float64{},
			SampledAt:  time.Now(),
		}
		p.mu.Unlock()
		return
	}

	// Sample keys from local HebbianStore.
	var sampledKeys [][16]byte
	if p.store != nil {
		var err error
		sampledKeys, err = p.store.SampleKeys(p.sampleN)
		if err != nil {
			slog.Warn("ccs: failed to sample keys", "err", err)
			return
		}
	}

	// Compute local hash.
	localHash, err := p.computeLocalHash(sampledKeys)
	if err != nil {
		slog.Warn("ccs: failed to compute local hash", "err", err)
		return
	}

	rid := uuid.New().String()
	probeMsg := mbp.CCSProbeMsg{
		SampledKeys: sampledKeys,
		RequestID:   rid,
	}
	payload, err := msgpack.Marshal(probeMsg)
	if err != nil {
		slog.Warn("ccs: failed to marshal probe msg", "err", err)
		return
	}

	p.pendingMu.Lock()
	p.pending[rid] = make(chan mbp.CCSResponseMsg, len(lobes))
	combinedCh := p.pending[rid]
	p.pendingMu.Unlock()

	defer func() {
		// Remove the pending entry so late-arriving HandleCCSResponse calls are
		// discarded rather than accumulating in a channel nobody drains.
		p.pendingMu.Lock()
		delete(p.pending, rid)
		p.pendingMu.Unlock()
		// Drain any buffered responses that arrived after we stopped waiting.
		for {
			select {
			case <-combinedCh:
			default:
				return
			}
		}
	}()

	// Send TypeCCSProbe to all lobes.
	for _, lobe := range lobes {
		peer, ok := p.coord.mgr.GetPeer(lobe.NodeID)
		if !ok {
			continue
		}
		_ = peer.Send(mbp.TypeCCSProbe, payload)
	}

	// Wait for responses with a 5-second timeout.
	deadline := time.Now().Add(5 * time.Second)
	responses := make(map[string]mbp.CCSResponseMsg)

	waitCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	for len(responses) < len(lobes) {
		select {
		case <-waitCtx.Done():
			goto compute
		case resp := <-combinedCh:
			responses[resp.NodeID] = resp
		}
	}

compute:
	// Compute per-node scores and overall score.
	nodeScores := make(map[string]float64, len(lobes))
	matched := 0

	for _, lobe := range lobes {
		resp, ok := responses[lobe.NodeID]
		if !ok {
			// No response → treat as inconsistent.
			nodeScores[lobe.NodeID] = 0.0
			continue
		}
		if hashesEqual(localHash, resp.Hash) {
			nodeScores[lobe.NodeID] = 1.0
			matched++
		} else {
			nodeScores[lobe.NodeID] = 0.0
		}
	}

	var score float64
	if len(lobes) > 0 {
		score = float64(matched) / float64(len(lobes))
	} else {
		score = 1.0
	}

	result := CCSResult{
		Score:      score,
		Assessment: ccsAssessment(score),
		NodeScores: nodeScores,
		SampledAt:  time.Now(),
	}

	p.mu.Lock()
	p.last = result
	p.mu.Unlock()

	slog.Debug("ccs: probe complete",
		"score", fmt.Sprintf("%.4f", score),
		"assessment", result.Assessment,
		"lobes", len(lobes),
		"matched", matched,
	)
}

// HandleCCSResponse is called when a TypeCCSResponse frame arrives from a Lobe.
// It delivers the response to the waiting probe goroutine.
func (p *CCSProbe) HandleCCSResponse(fromNodeID string, payload []byte) error {
	var resp mbp.CCSResponseMsg
	if err := msgpack.Unmarshal(payload, &resp); err != nil {
		return fmt.Errorf("ccs: unmarshal CCSResponse: %w", err)
	}

	p.pendingMu.Lock()
	ch, ok := p.pending[resp.RequestID]
	if !ok {
		// Response arrived after timeout — discard.
		p.pendingMu.Unlock()
		return nil
	}
	select {
	case ch <- resp:
	default:
		// Channel full (duplicate response) — discard.
	}
	p.pendingMu.Unlock()
	return nil
}

// HandleCCSProbe is called when a TypeCCSProbe frame arrives on a Lobe.
// It computes the local hash and sends back a TypeCCSResponse.
func (p *CCSProbe) HandleCCSProbe(fromNodeID string, payload []byte) error {
	var probe mbp.CCSProbeMsg
	if err := msgpack.Unmarshal(payload, &probe); err != nil {
		return fmt.Errorf("ccs: unmarshal CCSProbe: %w", err)
	}

	var hash []byte
	keyCount := 0

	if p.store != nil {
		localHash, err := p.computeLocalHash(probe.SampledKeys)
		if err != nil {
			slog.Warn("ccs: lobe failed to compute hash", "err", err)
		} else {
			hash = localHash
		}
		keyCount = len(probe.SampledKeys)
	}

	resp := mbp.CCSResponseMsg{
		RequestID: probe.RequestID,
		NodeID:    p.coord.cfg.NodeID,
		Hash:      hash,
		KeyCount:  keyCount,
	}
	respPayload, err := msgpack.Marshal(resp)
	if err != nil {
		return fmt.Errorf("ccs: marshal CCSResponse: %w", err)
	}

	peer, ok := p.coord.mgr.GetPeer(fromNodeID)
	if !ok {
		return nil // Cortex unreachable — best effort
	}
	return peer.Send(mbp.TypeCCSResponse, respPayload)
}

// computeLocalHash computes a SHA-256 hash over the (key, weight) pairs for
// the given keys, sorted by key for determinism.
func (p *CCSProbe) computeLocalHash(keys [][16]byte) ([]byte, error) {
	if len(keys) == 0 {
		// Empty key set: return hash of empty input.
		h := sha256.New()
		return h.Sum(nil), nil
	}

	var weights map[[16]byte]float64
	if p.store != nil {
		var err error
		weights, err = p.store.GetAssocWeightsForKeys(keys)
		if err != nil {
			return nil, err
		}
	} else {
		weights = make(map[[16]byte]float64)
	}

	return ComputeCCSHash(keys, weights), nil
}

// ComputeCCSHash computes a SHA-256 hash over sorted (key, weight) pairs.
// Exported for use in tests.
func ComputeCCSHash(keys [][16]byte, weights map[[16]byte]float64) []byte {
	// Sort keys for determinism.
	sorted := make([][16]byte, len(keys))
	copy(sorted, keys)
	sort.Slice(sorted, func(i, j int) bool {
		for b := 0; b < 16; b++ {
			if sorted[i][b] != sorted[j][b] {
				return sorted[i][b] < sorted[j][b]
			}
		}
		return false
	})

	h := sha256.New()
	var buf [24]byte // 16 bytes key + 8 bytes float64 weight
	for _, k := range sorted {
		copy(buf[:16], k[:])
		w := weights[k] // zero if not found
		binary.BigEndian.PutUint64(buf[16:], math64Bits(w))
		h.Write(buf[:])
	}
	return h.Sum(nil)
}

// math64Bits returns the IEEE 754 bit pattern for f.
func math64Bits(f float64) uint64 {
	return math.Float64bits(f)
}

// hashesEqual returns true if a and b are equal byte slices.
func hashesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
