package muninn

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultTimeout      = 5 * time.Second
	defaultMaxRetries   = 3
	defaultRetryBackoff = 500 * time.Millisecond
)

// Client is the MuninnDB REST API client.
type Client struct {
	baseURL       string
	token         string
	httpClient    *http.Client
	timeout       time.Duration
	maxRetries    int
	retryBackoff  time.Duration
}

// NewClient creates a new MuninnDB client.
func NewClient(baseURL, token string) *Client {
	return NewClientWithOptions(baseURL, token, defaultTimeout, defaultMaxRetries, defaultRetryBackoff)
}

// NewClientWithOptions creates a new MuninnDB client with custom options.
func NewClientWithOptions(baseURL, token string, timeout time.Duration, maxRetries int, retryBackoff time.Duration) *Client {
	return &Client{
		baseURL:      strings.TrimRight(baseURL, "/"),
		token:        token,
		httpClient:   &http.Client{Timeout: timeout},
		timeout:      timeout,
		maxRetries:   maxRetries,
		retryBackoff: retryBackoff,
	}
}

// Write writes an engram to the vault.
func (c *Client) Write(ctx context.Context, vault, concept, content string, tags []string) (string, error) {
	req := WriteRequest{
		Vault:      vault,
		Concept:    concept,
		Content:    content,
		Tags:       tags,
		Confidence: 0.9,
		Stability:  0.5,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	var resp WriteResponse
	if err := c.request(ctx, "POST", "/api/engrams", body, &resp); err != nil {
		return "", err
	}

	return resp.ID, nil
}

// WriteWithOptions writes an engram with full control over all fields.
func (c *Client) WriteWithOptions(ctx context.Context, req WriteRequest) (*WriteResponse, error) {
	if req.Confidence == 0 {
		req.Confidence = 0.9
	}
	if req.Stability == 0 {
		req.Stability = 0.5
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	var resp WriteResponse
	if err := c.request(ctx, "POST", "/api/engrams", body, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// WriteBatch writes multiple engrams in a single batch call. Maximum 50 per batch.
func (c *Client) WriteBatch(ctx context.Context, vault string, engrams []WriteRequest) (*BatchWriteResponse, error) {
	if len(engrams) == 0 {
		return nil, fmt.Errorf("engrams list must not be empty")
	}
	if len(engrams) > 50 {
		return nil, fmt.Errorf("batch size exceeds maximum of 50")
	}

	for i := range engrams {
		if engrams[i].Vault == "" {
			engrams[i].Vault = vault
		}
	}

	payload := struct {
		Engrams []WriteRequest `json:"engrams"`
	}{Engrams: engrams}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	var resp BatchWriteResponse
	if err := c.request(ctx, "POST", "/api/engrams/batch", body, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// Read reads an engram by ID.
func (c *Client) Read(ctx context.Context, id, vault string) (*Engram, error) {
	q := url.Values{}
	q.Set("vault", vault)
	path := fmt.Sprintf("/api/engrams/%s?%s", id, q.Encode())

	engram := &Engram{}
	if err := c.request(ctx, "GET", path, nil, engram); err != nil {
		return nil, err
	}

	return engram, nil
}

// Activate activates memory based on context.
func (c *Client) Activate(ctx context.Context, vault string, context []string, maxResults int) (*ActivateResponse, error) {
	req := ActivateRequest{
		Vault:      vault,
		Context:    context,
		MaxResults: maxResults,
		Threshold:  0.1,
		MaxHops:    0,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp := &ActivateResponse{}
	if err := c.request(ctx, "POST", "/api/activate", body, resp); err != nil {
		return nil, err
	}

	return resp, nil
}

// Link links two engrams. May not be supported in all MuninnDB versions.
func (c *Client) Link(ctx context.Context, vault, sourceID, targetID string, relType int, weight float64) error {
	req := LinkRequest{
		Vault:    vault,
		SourceID: sourceID,
		TargetID: targetID,
		RelType:  relType,
		Weight:   weight,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Try various possible endpoints
	endpoints := []string{
		"/api/links",
		"/api/engrams/link",
		fmt.Sprintf("/api/engrams/%s/links", sourceID),
	}

	var lastErr error
	for _, endpoint := range endpoints {
		err := c.request(ctx, "POST", endpoint, body, nil)
		if err == nil {
			return nil
		}
		lastErr = err
	}

	return fmt.Errorf("link endpoint not found: %w", lastErr)
}

// Forget forgets an engram.
func (c *Client) Forget(ctx context.Context, id, vault string) error {
	q := url.Values{}
	q.Set("vault", vault)
	path := fmt.Sprintf("/api/engrams/%s?%s", id, q.Encode())

	return c.request(ctx, "DELETE", path, nil, nil)
}

// Stats gets database statistics.
func (c *Client) Stats(ctx context.Context) (*StatsResponse, error) {
	resp := &StatsResponse{}
	if err := c.request(ctx, "GET", "/api/stats", nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// Subscribe subscribes to vault events via Server-Sent Events.
func (c *Client) Subscribe(ctx context.Context, vault string) (<-chan Push, error) {
	q := url.Values{}
	q.Set("vault", vault)
	q.Set("push_on_write", "true")
	url := fmt.Sprintf("%s/api/subscribe?%s", c.baseURL, q.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.addHeaders(req)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to SSE stream: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("subscription failed with status %d", resp.StatusCode)
	}

	ch := make(chan Push)

	go func() {
		defer func() {
			resp.Body.Close()
			close(ch)
		}()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Bytes()
			if !bytes.HasPrefix(line, []byte("data: ")) {
				continue
			}

			data := string(line[6:])
			var push Push
			if err := json.Unmarshal([]byte(data), &push); err != nil {
				continue
			}

			select {
			case ch <- push:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}

// Health checks if the server is healthy.
func (c *Client) Health(ctx context.Context) (bool, error) {
	resp := &StatsResponse{}
	if err := c.request(ctx, "GET", "/api/stats", nil, resp); err != nil {
		return false, err
	}
	return resp.EngramCount >= 0, nil
}

// request makes an HTTP request with automatic retry logic.
func (c *Client) request(ctx context.Context, method, path string, body []byte, result interface{}) error {
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		// Create request
		url := c.baseURL + path
		var req *http.Request
		var err error

		if body != nil {
			req, err = http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
		} else {
			req, err = http.NewRequestWithContext(ctx, method, url, nil)
		}

		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		c.addHeaders(req)

		// Send request
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt < c.maxRetries && c.isRetryable(err) {
				c.backoff(attempt)
				continue
			}
			return fmt.Errorf("request failed: %w", err)
		}

		// Handle response
		defer resp.Body.Close()

		if resp.StatusCode >= 500 && attempt < c.maxRetries {
			// Retry on 5xx errors
			lastErr = fmt.Errorf("server error %d", resp.StatusCode)
			c.backoff(attempt)
			continue
		}

		// Check for errors
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			respBody, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(respBody))
		}

		// Parse response
		if result != nil {
			if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
				return fmt.Errorf("failed to decode response: %w", err)
			}
		}

		return nil
	}

	if lastErr != nil {
		return fmt.Errorf("max retries exceeded: %w", lastErr)
	}
	return fmt.Errorf("max retries exceeded")
}

// isRetryable checks if an error is retryable.
func (c *Client) isRetryable(err error) bool {
	// Network errors are retryable
	return strings.Contains(err.Error(), "connection") ||
		strings.Contains(err.Error(), "timeout") ||
		strings.Contains(err.Error(), "temporary failure")
}

// backoff waits with exponential backoff + jitter.
func (c *Client) backoff(attempt int) {
	if attempt > 10 {
		attempt = 10 // Cap exponent to avoid huge waits
	}
	delay := time.Duration(math.Pow(2, float64(attempt))) * c.retryBackoff
	jitter := time.Duration(rand.Intn(100)) * time.Millisecond
	time.Sleep(delay + jitter)
}

// addHeaders adds default headers to the request.
func (c *Client) addHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	}
}
