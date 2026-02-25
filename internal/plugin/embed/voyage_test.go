package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVoyageProvider_Init_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/v1/embeddings" {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer test-key" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []map[string]interface{}{
					{"embedding": []float32{0.1, 0.2, 0.3}},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &VoyageProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "voyage-3",
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

func TestVoyageProvider_Init_NoAPIKey(t *testing.T) {
	provider := &VoyageProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "https://api.voyageai.com",
		Model:   "voyage-3",
		APIKey:  "",
	}

	_, err := provider.Init(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestVoyageProvider_Embed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/v1/embeddings" {
			var req voyageEmbedRequest
			json.NewDecoder(r.Body).Decode(&req)

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []map[string]interface{}{
					{"embedding": []float32{0.1, 0.2}, "index": 0},
					{"embedding": []float32{0.3, 0.4}, "index": 1},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &VoyageProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "voyage-3",
		APIKey:  "test-key",
	}

	provider.Init(context.Background(), cfg)

	result, err := provider.EmbedBatch(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}

	expectedLen := 4 // 2 texts * 2 dimension
	if len(result) != expectedLen {
		t.Errorf("expected %d embeddings, got %d", expectedLen, len(result))
	}
}

func TestVoyageProvider_MaxBatchSize(t *testing.T) {
	provider := &VoyageProvider{}
	if provider.MaxBatchSize() != 128 {
		t.Errorf("expected batch size 128, got %d", provider.MaxBatchSize())
	}
}

func TestVoyageProvider_Close(t *testing.T) {
	provider := &VoyageProvider{}
	err := provider.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestVoyageProvider_Name(t *testing.T) {
	provider := &VoyageProvider{}
	if provider.Name() != "voyage" {
		t.Errorf("expected name voyage, got %s", provider.Name())
	}
}

func TestVoyageProvider_Embed_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/v1/embeddings" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error": "invalid API key"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &VoyageProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "voyage-3",
		APIKey:  "bad-key",
	}

	provider.Init(context.Background(), cfg)

	_, err := provider.EmbedBatch(context.Background(), []string{"test"})
	if err == nil {
		t.Fatal("expected error for unauthorized")
	}
}

func TestVoyageProvider_Init_NoData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/v1/embeddings" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []map[string]interface{}{},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &VoyageProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "voyage-3",
		APIKey:  "test-key",
	}

	_, err := provider.Init(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for no data")
	}
}

func TestVoyageProvider_Init_EmptyEmbedding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/v1/embeddings" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []map[string]interface{}{
					{"embedding": []float32{}, "index": 0},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &VoyageProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "voyage-3",
		APIKey:  "test-key",
	}

	_, err := provider.Init(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for empty embedding")
	}
}

func TestVoyageProvider_Init_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/v1/embeddings" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("not valid json"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &VoyageProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "voyage-3",
		APIKey:  "test-key",
	}

	_, err := provider.Init(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestVoyageProvider_Init_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/v1/embeddings" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("server error"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &VoyageProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "voyage-3",
		APIKey:  "test-key",
	}

	_, err := provider.Init(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for server error")
	}
}

func TestVoyageProvider_EmbedBatch_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/v1/embeddings" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("not json"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &VoyageProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "voyage-3",
		APIKey:  "test-key",
	}
	provider.Init(context.Background(), cfg)

	_, err := provider.EmbedBatch(context.Background(), []string{"test"})
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestVoyageProvider_EmbedBatch_ServerError(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/v1/embeddings" {
			callCount++
			if callCount == 1 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"data": []map[string]interface{}{
						{"embedding": []float32{0.1, 0.2, 0.3}, "index": 0},
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

	provider := &VoyageProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "voyage-3",
		APIKey:  "test-key",
	}
	provider.Init(context.Background(), cfg)

	_, err := provider.EmbedBatch(context.Background(), []string{"test"})
	if err == nil {
		t.Fatal("expected error for server error")
	}
}

func TestVoyageProvider_EmbedBatch_ResultOrdering(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/v1/embeddings" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []map[string]interface{}{
					{"embedding": []float32{0.3, 0.4}, "index": 1},
					{"embedding": []float32{0.1, 0.2}, "index": 0},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &VoyageProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "voyage-3",
		APIKey:  "test-key",
	}
	provider.Init(context.Background(), cfg)

	result, err := provider.EmbedBatch(context.Background(), []string{"first", "second"})
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}

	if result[0] != 0.1 || result[1] != 0.2 {
		t.Errorf("expected first embedding [0.1, 0.2], got [%f, %f]", result[0], result[1])
	}
	if result[2] != 0.3 || result[3] != 0.4 {
		t.Errorf("expected second embedding [0.3, 0.4], got [%f, %f]", result[2], result[3])
	}
}

func TestVoyageProvider_Close_WithClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/v1/embeddings" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []map[string]interface{}{
					{"embedding": []float32{0.1, 0.2, 0.3}, "index": 0},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &VoyageProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://" + server.Listener.Addr().String(),
		Model:   "voyage-3",
		APIKey:  "test-key",
	}
	provider.Init(context.Background(), cfg)

	err := provider.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestVoyageProvider_Init_Unreachable(t *testing.T) {
	provider := &VoyageProvider{}
	cfg := ProviderHTTPConfig{
		BaseURL: "http://localhost:54321",
		Model:   "voyage-3",
		APIKey:  "test-key",
	}

	_, err := provider.Init(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}
