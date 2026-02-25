package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCohereProvider_Init_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/v2/embed" {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer test-key" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"embeddings": map[string]interface{}{
					"float": [][]float32{{0.1, 0.2, 0.3}},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &CohereProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "embed-v4",
		APIKey:  "test-key",
	}

	dim, err := provider.Init(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if dim != 3 {
		t.Errorf("expected dimension 3, got %d", dim)
	}
}

func TestCohereProvider_Init_NoAPIKey(t *testing.T) {
	provider := &CohereProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "https://api.cohere.com",
		Model:   "embed-v4",
		APIKey:  "",
	}

	_, err := provider.Init(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestCohereProvider_Embed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/v2/embed" {
			var req cohereEmbedRequest
			json.NewDecoder(r.Body).Decode(&req)

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"embeddings": map[string]interface{}{
					"float": [][]float32{{0.1, 0.2}, {0.3, 0.4}},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &CohereProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "embed-v4",
		APIKey:  "test-key",
	}

	provider.Init(context.Background(), cfg)

	result, err := provider.EmbedBatch(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}

	expectedLen := 4
	if len(result) != expectedLen {
		t.Errorf("expected %d embeddings, got %d", expectedLen, len(result))
	}
}

func TestCohereProvider_MaxBatchSize(t *testing.T) {
	provider := &CohereProvider{}
	if provider.MaxBatchSize() != 96 {
		t.Errorf("expected batch size 96, got %d", provider.MaxBatchSize())
	}
}

func TestCohereProvider_Close(t *testing.T) {
	provider := &CohereProvider{}
	err := provider.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestCohereProvider_Name(t *testing.T) {
	provider := &CohereProvider{}
	if provider.Name() != "cohere" {
		t.Errorf("expected name cohere, got %s", provider.Name())
	}
}

func TestCohereProvider_Embed_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/v2/embed" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error": "invalid API key"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &CohereProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "embed-v4",
		APIKey:  "bad-key",
	}

	provider.Init(context.Background(), cfg)

	_, err := provider.EmbedBatch(context.Background(), []string{"test"})
	if err == nil {
		t.Fatal("expected error for unauthorized")
	}
}

func TestCohereProvider_Init_NoData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/v2/embed" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"embeddings": map[string]interface{}{
					"float": [][]float32{},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &CohereProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "embed-v4",
		APIKey:  "test-key",
	}

	_, err := provider.Init(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for no data")
	}
}

func TestCohereProvider_Init_EmptyEmbedding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/v2/embed" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"embeddings": map[string]interface{}{
					"float": [][]float32{{}},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &CohereProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "embed-v4",
		APIKey:  "test-key",
	}

	_, err := provider.Init(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for empty embedding")
	}
}

func TestCohereProvider_Init_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/v2/embed" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("not valid json"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &CohereProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "embed-v4",
		APIKey:  "test-key",
	}

	_, err := provider.Init(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestCohereProvider_Init_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/v2/embed" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("server error"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &CohereProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "embed-v4",
		APIKey:  "test-key",
	}

	_, err := provider.Init(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for server error")
	}
}

func TestCohereProvider_Init_RateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/v2/embed" {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"message": "rate limit exceeded"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &CohereProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "embed-v4",
		APIKey:  "test-key",
	}

	_, err := provider.Init(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for rate limit")
	}
}

func TestCohereProvider_EmbedBatch_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/v2/embed" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("not json"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &CohereProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "embed-v4",
		APIKey:  "test-key",
	}
	provider.Init(context.Background(), cfg)

	_, err := provider.EmbedBatch(context.Background(), []string{"test"})
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestCohereProvider_EmbedBatch_ServerError(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/v2/embed" {
			callCount++
			if callCount == 1 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"embeddings": map[string]interface{}{
						"float": [][]float32{{0.1, 0.2, 0.3}},
					},
				})
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("server error"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &CohereProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "embed-v4",
		APIKey:  "test-key",
	}
	provider.Init(context.Background(), cfg)

	_, err := provider.EmbedBatch(context.Background(), []string{"test"})
	if err == nil {
		t.Fatal("expected error for server error")
	}
}

func TestCohereProvider_Init_Unreachable(t *testing.T) {
	provider := &CohereProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://localhost:54321",
		Model:   "embed-v4",
		APIKey:  "test-key",
	}

	_, err := provider.Init(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

func TestCohereProvider_Close_WithClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/v2/embed" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"embeddings": map[string]interface{}{
					"float": [][]float32{{0.1, 0.2, 0.3}},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &CohereProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "embed-v4",
		APIKey:  "test-key",
	}
	provider.Init(context.Background(), cfg)

	err := provider.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}
