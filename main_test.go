package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(10, 5)
	defer rl.Close()
	for i := 0; i < 5; i++ {
		if !rl.Allow("192.168.1.1") {
			t.Fatalf("request %d should be allowed", i)
		}
	}
	if rl.Allow("192.168.1.1") {
		t.Fatal("sixth request should be denied")
	}
	time.Sleep(150 * time.Millisecond)
	if !rl.Allow("192.168.1.1") {
		t.Fatal("request should be allowed after refill")
	}
}

func TestRateLimiterRejectsInvalidConfiguration(t *testing.T) {
	for _, test := range []struct{ rate, capacity, maxVisitors int }{{0, 1, 1}, {1, 0, 1}, {1, 1, 0}, {-1, 1, 1}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("configuration %+v should panic", test)
				}
			}()
			NewRateLimiterWithLimit(test.rate, test.capacity, test.maxVisitors)
		}()
	}
}

func TestRateLimiterObserverAndStats(t *testing.T) {
	rl := NewRateLimiter(1, 1)
	defer rl.Close()
	var allowed, denied atomic.Int64
	rl.SetObserver(func(_ string, ok bool, _ float64) {
		if ok {
			allowed.Add(1)
		} else {
			denied.Add(1)
		}
	})
	if !rl.Allow("203.0.113.10") || rl.Allow("203.0.113.10") {
		t.Fatal("expected allow then deny")
	}
	if allowed.Load() != 1 || denied.Load() != 1 {
		t.Fatalf("observer allowed=%d denied=%d", allowed.Load(), denied.Load())
	}
	stats := rl.Stats()
	if stats.Allowed != 1 || stats.Denied != 1 || stats.ActiveVisitors != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestMaxVisitorBound(t *testing.T) {
	rl := NewRateLimiterWithLimit(10, 2, 1)
	defer rl.Close()
	if !rl.Allow("first") {
		t.Fatal("first visitor should be allowed")
	}
	decision := rl.Take("second")
	if decision.Allowed {
		t.Fatal("second visitor should be denied when visitor bound is full")
	}
	if rl.Stats().ActiveVisitors != 1 {
		t.Fatal("visitor map exceeded configured bound")
	}
}

func TestDecisionRetryAfter(t *testing.T) {
	rl := NewRateLimiter(2, 1)
	defer rl.Close()
	if !rl.Take("client").Allowed {
		t.Fatal("first decision should allow")
	}
	decision := rl.Take("client")
	if decision.Allowed || decision.RetryAfterMS <= 0 {
		t.Fatalf("unexpected denial decision: %+v", decision)
	}
}

func TestRequestIdentity(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	req.RemoteAddr = "192.0.2.10:1234"
	identity, err := requestIdentity(req, "")
	if err != nil || identity != "192.0.2.10" {
		t.Fatalf("identity=%q err=%v", identity, err)
	}
	req.Header.Set("X-API-Key", "tenant-a")
	identity, err = requestIdentity(req, "X-API-Key")
	if err != nil || identity != "tenant-a" {
		t.Fatalf("header identity=%q err=%v", identity, err)
	}
}

func TestRequestIdentityRequiresConfiguredHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	if _, err := requestIdentity(req, "X-API-Key"); err == nil {
		t.Fatal("missing configured identity header should fail")
	}
}

func TestMiddlewareReturns429AndRetryAfter(t *testing.T) {
	rl := NewRateLimiter(1, 1)
	defer rl.Close()
	handler := limitMiddleware(rl, "X-Client", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	for i, want := range []int{http.StatusNoContent, http.StatusTooManyRequests} {
		req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
		req.Header.Set("X-Client", "tenant")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != want {
			t.Fatalf("request %d status=%d want=%d", i, rr.Code, want)
		}
		if i == 1 && rr.Header().Get("Retry-After") == "" {
			t.Fatal("denial must expose Retry-After")
		}
	}
}

func TestCheckHandler(t *testing.T) {
	rl := NewRateLimiter(1, 1)
	defer rl.Close()
	handler := checkHandler(rl)
	req := httptest.NewRequest(http.MethodPost, "/v1/check", strings.NewReader(`{"key":"api-client"}`))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var decision Decision
	if err := json.Unmarshal(rr.Body.Bytes(), &decision); err != nil {
		t.Fatal(err)
	}
	if !decision.Allowed {
		t.Fatal("first decision should be allowed")
	}
}

func TestCheckHandlerRejectsOversizedOrMalformedInput(t *testing.T) {
	rl := NewRateLimiter(1, 1)
	defer rl.Close()
	handler := checkHandler(rl)
	for _, body := range []string{`{}`, `{"key":""}`, `{"key":"ok","extra":true}`} {
		req := httptest.NewRequest(http.MethodPost, "/v1/check", strings.NewReader(body))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("body=%s status=%d", body, rr.Code)
		}
	}
}

func TestHealthAndMetricsHandlers(t *testing.T) {
	rl := NewRateLimiter(10, 2)
	defer rl.Close()
	rl.Allow("client")
	for _, handler := range []http.HandlerFunc{healthHandler, metricsHandler(rl)} {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
		if rr.Code != http.StatusOK || rr.Header().Get("Content-Type") != "application/json" {
			t.Fatalf("unexpected response: %d %q", rr.Code, rr.Header().Get("Content-Type"))
		}
	}
}

func TestRateLimiterCloseIsIdempotent(t *testing.T) {
	rl := NewRateLimiter(1, 1)
	rl.Close()
	rl.Close()
}

func TestClientIP(t *testing.T) {
	if got := clientIP("192.0.2.10:1234"); got != "192.0.2.10" {
		t.Fatalf("got %q", got)
	}
	if got := clientIP("198.51.100.10"); got != "198.51.100.10" {
		t.Fatalf("got %q", got)
	}
}
