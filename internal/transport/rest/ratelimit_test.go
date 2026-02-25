package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/time/rate"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func newTestIPCache(t *testing.T) *lru.Cache[string, *rate.Limiter] {
	t.Helper()
	c, err := lru.New[string, *rate.Limiter](64)
	if err != nil {
		t.Fatalf("lru.New: %v", err)
	}
	return c
}

func TestRateLimitMiddleware_GlobalLimitRejects(t *testing.T) {
	global := rate.NewLimiter(1, 1) // 1 RPS, burst 1
	mw := newRateLimitMiddleware(global, newTestIPCache(t), 1000)
	handler := mw(okHandler())

	// First request consumes the burst token.
	r1 := httptest.NewRequest(http.MethodGet, "/", nil)
	r1.RemoteAddr = "10.0.0.1:1234"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, r1)

	if w1.Code != http.StatusOK {
		t.Fatalf("first request: want 200, got %d", w1.Code)
	}

	// Second request should be rejected immediately.
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2.RemoteAddr = "10.0.0.1:1234"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, r2)

	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: want 429, got %d", w2.Code)
	}
}

func TestRateLimitMiddleware_GlobalLimitAllows(t *testing.T) {
	global := rate.NewLimiter(1000, 1000)
	mw := newRateLimitMiddleware(global, newTestIPCache(t), 1000)
	handler := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
}

func TestRateLimitMiddleware_PerIPLimitRejects(t *testing.T) {
	global := rate.NewLimiter(10000, 10000)
	// perIPRPS=1 → burst = 1*2 = 2 (set by middleware: perIPRPS*2)
	mw := newRateLimitMiddleware(global, newTestIPCache(t), 1)
	handler := mw(okHandler())

	ip := "10.0.0.99:5555"
	// Exhaust the per-IP burst (burst = 2).
	for i := 0; i < 2; i++ {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = ip
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: want 200, got %d", i+1, w.Code)
		}
	}

	// Next request from the same IP should be rejected.
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = ip
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429 from per-IP limiter, got %d", w.Code)
	}
}

func TestRateLimitMiddleware_DifferentIPsIndependent(t *testing.T) {
	global := rate.NewLimiter(10000, 10000)
	mw := newRateLimitMiddleware(global, newTestIPCache(t), 1)
	handler := mw(okHandler())

	for _, ip := range []string{"10.0.0.1:1000", "10.0.0.2:2000"} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = ip
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("IP %s: want 200, got %d", ip, w.Code)
		}
	}
}

func TestRateLimitMiddleware_RetryAfterHeader(t *testing.T) {
	global := rate.NewLimiter(1, 1)
	mw := newRateLimitMiddleware(global, newTestIPCache(t), 1000)
	handler := mw(okHandler())

	// Consume the burst.
	r1 := httptest.NewRequest(http.MethodGet, "/", nil)
	r1.RemoteAddr = "10.0.0.1:1234"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, r1)

	// Trigger 429.
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2.RemoteAddr = "10.0.0.1:1234"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, r2)

	ra := w2.Header().Get("Retry-After")
	if ra == "" {
		t.Fatal("Retry-After header missing on 429 response")
	}
	secs, err := strconv.Atoi(ra)
	if err != nil {
		t.Fatalf("Retry-After is not a valid integer: %q", ra)
	}
	if secs < 1 {
		t.Fatalf("Retry-After should be >= 1, got %d", secs)
	}
}

func TestRateLimitMiddleware_ResponseBody429(t *testing.T) {
	global := rate.NewLimiter(1, 1)
	mw := newRateLimitMiddleware(global, newTestIPCache(t), 1000)
	handler := mw(okHandler())

	// Consume the burst.
	r1 := httptest.NewRequest(http.MethodGet, "/", nil)
	r1.RemoteAddr = "10.0.0.1:1234"
	handler.ServeHTTP(httptest.NewRecorder(), r1)

	// Trigger 429.
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2.RemoteAddr = "10.0.0.1:1234"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, r2)

	var body APIError
	if err := json.NewDecoder(w2.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body.Code != "rate_limit_exceeded" {
		t.Fatalf("code: want %q, got %q", "rate_limit_exceeded", body.Code)
	}
	if body.Message != "rate limit exceeded" {
		t.Fatalf("message: want %q, got %q", "rate limit exceeded", body.Message)
	}
}

func TestClientIP_IgnoresXForwardedFor(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.0.2.1:54321"
	r.Header.Set("X-Forwarded-For", "203.0.113.50")
	if got := clientIP(r); got != "192.0.2.1" {
		t.Fatalf("X-Forwarded-For should be ignored; want 192.0.2.1, got %s", got)
	}
}

func TestClientIP_IgnoresXRealIP(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.0.2.1:54321"
	r.Header.Set("X-Real-IP", "198.51.100.22")
	if got := clientIP(r); got != "192.0.2.1" {
		t.Fatalf("X-Real-IP should be ignored; want 192.0.2.1, got %s", got)
	}
}

func TestClientIP_RemoteAddr(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.0.2.1:54321"
	if got := clientIP(r); got != "192.0.2.1" {
		t.Fatalf("want 192.0.2.1, got %s", got)
	}
}

func TestClientIP_RemoteAddrIPv6(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "[::1]:8080"
	if got := clientIP(r); got != "::1" {
		t.Fatalf("want ::1, got %s", got)
	}
}

func TestRateLimitMiddleware_RecoveryAfterBurst(t *testing.T) {
	// 100 RPS → token every 10ms; burst 1.
	global := rate.NewLimiter(100, 1)
	mw := newRateLimitMiddleware(global, newTestIPCache(t), 1000)
	handler := mw(okHandler())

	// Consume the burst.
	r1 := httptest.NewRequest(http.MethodGet, "/", nil)
	r1.RemoteAddr = "10.0.0.1:1234"
	handler.ServeHTTP(httptest.NewRecorder(), r1)

	// Verify rejection.
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2.RemoteAddr = "10.0.0.1:1234"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, r2)
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429 immediately after burst, got %d", w2.Code)
	}

	// Wait for a token to refill (~10ms at 100 RPS).
	time.Sleep(50 * time.Millisecond)

	// Should pass now.
	r3 := httptest.NewRequest(http.MethodGet, "/", nil)
	r3.RemoteAddr = "10.0.0.1:1234"
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, r3)
	if w3.Code != http.StatusOK {
		t.Fatalf("want 200 after recovery, got %d", w3.Code)
	}
}
