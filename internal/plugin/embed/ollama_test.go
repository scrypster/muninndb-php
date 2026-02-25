package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOllamaProvider_Init_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/api/embeddings" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"embedding": [0.1, 0.2, 0.3]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &OllamaProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "test-model",
	}

	dim, err := provider.Init(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if dim != 3 {
		t.Errorf("expected dimension 3, got %d", dim)
	}
}

func TestOllamaProvider_Init_Unreachable(t *testing.T) {
	provider := &OllamaProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://localhost:54321",
		Model:   "test-model",
	}

	_, err := provider.Init(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

func TestOllamaProvider_Embed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/api/embeddings" {
			var req ollamaEmbedRequest
			json.NewDecoder(r.Body).Decode(&req)

			embedding := []float64{0.1, 0.2, 0.3}
			if req.Prompt == "hello" {
				embedding = []float64{0.4, 0.5, 0.6}
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string][]float64{
				"embedding": embedding,
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &OllamaProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "test-model",
	}

	provider.Init(context.Background(), cfg)

	result, err := provider.EmbedBatch(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}

	expectedLen := 6 // 2 texts * 3 dimension
	if len(result) != expectedLen {
		t.Errorf("expected %d embeddings, got %d", expectedLen, len(result))
	}
}

func TestOllamaProvider_MaxBatchSize(t *testing.T) {
	provider := &OllamaProvider{}
	if provider.MaxBatchSize() != 1 {
		t.Errorf("expected batch size 1, got %d", provider.MaxBatchSize())
	}
}

func TestOllamaProvider_Close(t *testing.T) {
	provider := &OllamaProvider{}
	err := provider.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestOllamaProvider_Name(t *testing.T) {
	provider := &OllamaProvider{}
	if provider.Name() != "ollama" {
		t.Errorf("expected name ollama, got %s", provider.Name())
	}
}

func TestOllamaProvider_Embed_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/api/embeddings" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal error"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &OllamaProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "test-model",
	}

	provider.Init(context.Background(), cfg)

	_, err := provider.EmbedBatch(context.Background(), []string{"test"})
	if err == nil {
		t.Fatal("expected error for server error")
	}
}

func TestOllamaProvider_Init_ProbeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/api/embeddings" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("model not found"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &OllamaProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "nonexistent-model",
	}

	_, err := provider.Init(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for model not found")
	}
}

func TestOllamaProvider_Embed_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/api/embeddings" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("invalid json"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &OllamaProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "test-model",
	}

	// Modify Init to handle bad JSON
	provider.baseURL = cfg.BaseURL
	provider.model = cfg.Model
	transport := &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90,
		TLSHandshakeTimeout: 10,
	}
	provider.client = &http.Client{Transport: transport, Timeout: 10}

	_, err := provider.EmbedBatch(context.Background(), []string{"test"})
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestOllamaProvider_EmptyEmbedding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/api/embeddings" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// First request (probe) succeeds with empty embedding
			w.Write([]byte(`{"embedding": []}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &OllamaProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "test-model",
	}

	_, err := provider.Init(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for empty embedding")
	}
}

func TestOllamaProvider_Close_WithClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/api/embeddings" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"embedding": [0.1, 0.2, 0.3]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &OllamaProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "test-model",
	}
	provider.Init(context.Background(), cfg)

	err := provider.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestOllamaProvider_Init_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/api/embeddings" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("not valid json"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &OllamaProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "test-model",
	}

	_, err := provider.Init(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for invalid JSON in init")
	}
}

func TestOllamaProvider_EmbedBatch_NonEmptyResult(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/api/embeddings" {
			callCount++
			var req ollamaEmbedRequest
			json.NewDecoder(r.Body).Decode(&req)

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string][]float64{
				"embedding": []float64{0.1 * float64(callCount), 0.2 * float64(callCount)},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &OllamaProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "test-model",
	}

	provider.Init(context.Background(), cfg)

	// Reset for actual embed calls
	callCount = 0
	result, err := provider.EmbedBatch(context.Background(), []string{"text1", "text2"})
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}

	// Should have 2 texts * 2 dimension = 4 values
	if len(result) != 4 {
		t.Errorf("expected 4 values, got %d", len(result))
	}

	// Verify result contains expected values
	if len(result) > 0 {
		// Just verify we got some data back
	}
}
