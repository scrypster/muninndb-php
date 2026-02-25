package rest

import (
	"context"
	"io"
	"time"

	"github.com/scrypster/muninndb/internal/cognitive"
	"github.com/scrypster/muninndb/internal/engine"
	"github.com/scrypster/muninndb/internal/engine/trigger"
	"github.com/scrypster/muninndb/internal/engine/vaultjob"
	hnswpkg "github.com/scrypster/muninndb/internal/index/hnsw"
	"github.com/scrypster/muninndb/internal/storage"
	mbp "github.com/scrypster/muninndb/internal/transport/mbp"
)

// RESTEngineWrapper wraps the Engine to adapt it for the REST interface.
// All methods accept a context and pass it through to the engine.
type RESTEngineWrapper struct {
	engine  *engine.Engine
	hnswReg *hnswpkg.Registry
}

// NewEngineWrapper returns an EngineAPI backed by eng with optional HNSW stat injection.
func NewEngineWrapper(eng *engine.Engine, hnswReg *hnswpkg.Registry) EngineAPI {
	return &RESTEngineWrapper{engine: eng, hnswReg: hnswReg}
}

func (w *RESTEngineWrapper) Hello(ctx context.Context, req *HelloRequest) (*HelloResponse, error) {
	return w.engine.Hello(ctx, req)
}

func (w *RESTEngineWrapper) Write(ctx context.Context, req *WriteRequest) (*WriteResponse, error) {
	return w.engine.Write(ctx, req)
}

func (w *RESTEngineWrapper) WriteBatch(ctx context.Context, reqs []*WriteRequest) ([]*WriteResponse, []error) {
	return w.engine.WriteBatch(ctx, reqs)
}

func (w *RESTEngineWrapper) Read(ctx context.Context, req *ReadRequest) (*ReadResponse, error) {
	return w.engine.Read(ctx, req)
}

// coerceFilterValues returns a new slice of filters where string values for
// temporal fields ("created_after", "created_before") are parsed into time.Time.
// Values that are already time.Time are left unchanged. If parsing fails the
// value is left as-is so the engine can handle or ignore it gracefully.
// The original slice is never mutated.
func coerceFilterValues(filters []mbp.Filter) []mbp.Filter {
	out := make([]mbp.Filter, len(filters))
	copy(out, filters)
	for i, f := range out {
		if f.Field != "created_after" && f.Field != "created_before" {
			continue
		}
		if _, ok := f.Value.(time.Time); ok {
			continue
		}
		s, ok := f.Value.(string)
		if !ok {
			continue
		}
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			out[i].Value = t
			continue
		}
		if t, err := time.Parse("2006-01-02", s); err == nil {
			out[i].Value = t
		}
	}
	return out
}

func (w *RESTEngineWrapper) Activate(ctx context.Context, req *ActivateRequest) (*ActivateResponse, error) {
	if len(req.Filters) > 0 {
		// Make a shallow copy to avoid mutating the caller's request.
		reqCopy := *req
		reqCopy.Filters = coerceFilterValues(req.Filters)
		req = &reqCopy
	}
	return w.engine.Activate(ctx, req)
}

func (w *RESTEngineWrapper) Link(ctx context.Context, req *mbp.LinkRequest) (*LinkResponse, error) {
	return w.engine.Link(ctx, req)
}

func (w *RESTEngineWrapper) Forget(ctx context.Context, req *ForgetRequest) (*ForgetResponse, error) {
	return w.engine.Forget(ctx, req)
}

func (w *RESTEngineWrapper) Stat(ctx context.Context, req *StatRequest) (*StatResponse, error) {
	resp, err := w.engine.Stat(ctx, req)
	if err != nil {
		return nil, err
	}
	if w.hnswReg != nil {
		if req.Vault != "" {
			ws := w.engine.Store().ResolveVaultPrefix(req.Vault)
			resp.IndexSize = w.hnswReg.VaultVectorBytes(ws)
		} else {
			resp.IndexSize = w.hnswReg.TotalVectorBytes()
		}
	}
	return resp, nil
}

func (w *RESTEngineWrapper) ListEngrams(ctx context.Context, req *ListEngramsRequest) (*ListEngramsResponse, error) {
	maxNeeded := req.Offset + req.Limit
	if maxNeeded <= 0 {
		maxNeeded = 20
	}
	aReq := &ActivateRequest{
		Context:    []string{},
		MaxResults: maxNeeded * 2,
		Vault:      req.Vault,
		Threshold:  0.0,
	}
	resp, err := w.engine.Activate(ctx, aReq)
	if err != nil {
		return nil, err
	}
	items := resp.Activations
	total := len(items)
	if req.Offset > len(items) {
		items = nil
	} else {
		items = items[req.Offset:]
	}
	if req.Limit > 0 && len(items) > req.Limit {
		items = items[:req.Limit]
	}
	engrams := make([]EngramItem, len(items))
	for i, a := range items {
		engrams[i] = EngramItem{
			ID:         a.ID,
			Concept:    a.Concept,
			Content:    a.Content,
			Confidence: a.Confidence,
			Vault:      req.Vault,
		}
	}
	return &ListEngramsResponse{
		Engrams: engrams,
		Total:   total,
		Limit:   req.Limit,
		Offset:  req.Offset,
	}, nil
}

func (w *RESTEngineWrapper) GetEngramLinks(ctx context.Context, req *GetEngramLinksRequest) (*GetEngramLinksResponse, error) {
	vault := req.Vault
	if vault == "" {
		vault = "default"
	}
	assocs, err := w.engine.GetAssociations(ctx, vault, req.ID, 50)
	if err != nil {
		return nil, err
	}
	links := make([]AssociationItem, len(assocs))
	for i, a := range assocs {
		links[i] = AssociationItem{
			TargetID: a.TargetID.String(),
			RelType:  uint16(a.RelType),
			Weight:   a.Weight,
		}
	}
	return &GetEngramLinksResponse{Links: links}, nil
}

func (w *RESTEngineWrapper) ListVaults(ctx context.Context) ([]string, error) {
	names, err := w.engine.ListVaults(ctx)
	if err != nil || len(names) == 0 {
		return []string{"default"}, nil
	}
	return names, nil
}

func (w *RESTEngineWrapper) GetSession(ctx context.Context, req *GetSessionRequest) (*GetSessionResponse, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}
	res, err := w.engine.SessionPaged(ctx, req.Vault, req.Since, offset, limit)
	if err != nil {
		return nil, err
	}
	entries := make([]SessionItem, 0, len(res.Writes))
	for _, wr := range res.Writes {
		entries = append(entries, SessionItem{
			ID:        wr.ID,
			Concept:   wr.Concept,
			CreatedAt: wr.At.Unix(),
		})
	}
	return &GetSessionResponse{
		Entries: entries,
		Total:   res.Total,
		Offset:  offset,
		Limit:   limit,
	}, nil
}

func (w *RESTEngineWrapper) WorkerStats() cognitive.EngineWorkerStats {
	return w.engine.WorkerStats()
}

func (w *RESTEngineWrapper) SubscribeWithDeliver(ctx context.Context, req *mbp.SubscribeRequest, deliver trigger.DeliverFunc) (string, error) {
	return w.engine.SubscribeWithDeliver(ctx, req, deliver)
}

func (w *RESTEngineWrapper) Unsubscribe(ctx context.Context, subID string) error {
	return w.engine.Unsubscribe(ctx, subID)
}

func (w *RESTEngineWrapper) CountEmbedded(ctx context.Context) int64 {
	return w.engine.CountEmbedded(ctx)
}

func (w *RESTEngineWrapper) RecordAccess(ctx context.Context, vault, id string) error {
	return w.engine.RecordAccess(ctx, vault, id)
}

func (w *RESTEngineWrapper) ClearVault(ctx context.Context, vaultName string) error {
	return w.engine.ClearVault(ctx, vaultName)
}

func (w *RESTEngineWrapper) DeleteVault(ctx context.Context, vaultName string) error {
	return w.engine.DeleteVault(ctx, vaultName)
}

func (w *RESTEngineWrapper) StartClone(ctx context.Context, sourceVault, newName string) (*vaultjob.Job, error) {
	return w.engine.StartClone(ctx, sourceVault, newName)
}

func (w *RESTEngineWrapper) StartMerge(ctx context.Context, sourceVault, targetVault string, deleteSource bool) (*vaultjob.Job, error) {
	return w.engine.StartMerge(ctx, sourceVault, targetVault, deleteSource)
}

func (w *RESTEngineWrapper) GetVaultJob(jobID string) (*vaultjob.Job, bool) {
	return w.engine.GetVaultJob(jobID)
}

func (w *RESTEngineWrapper) ExportVault(ctx context.Context, vaultName, embedderModel string, dimension int, resetMeta bool, wr io.Writer) (*storage.ExportResult, error) {
	return w.engine.ExportVault(ctx, vaultName, embedderModel, dimension, resetMeta, wr)
}

func (w *RESTEngineWrapper) StartImport(ctx context.Context, vaultName, embedderModel string, dimension int, resetMeta bool, r io.Reader) (*vaultjob.Job, error) {
	return w.engine.StartImport(ctx, vaultName, embedderModel, dimension, resetMeta, r)
}

func (w *RESTEngineWrapper) ReindexFTSVault(ctx context.Context, vaultName string) (int64, error) {
	return w.engine.ReindexFTSVault(ctx, vaultName)
}

func (w *RESTEngineWrapper) Checkpoint(destDir string) error {
	return w.engine.Checkpoint(destDir)
}
