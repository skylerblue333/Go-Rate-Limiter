package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Visitor stores fractional token-bucket state for one client identity.
type Visitor struct {
	tokens   float64
	lastSeen time.Time
}

// RateLimiterConfig controls lifecycle and memory bounds.
type RateLimiterConfig struct {
	Rate           int
	Capacity       int
	CleanupEvery   time.Duration
	IdleTTL        time.Duration
	MaxVisitors    int
}

// Decision is the observable admission result for a request.
type Decision struct {
	Allowed    bool
	Remaining  float64
	RetryAfter time.Duration
}

// RateLimiter is a process-local token-bucket limiter.
//
// It is intentionally deterministic and dependency-free. The memory bound,
// configurable cleanup, and explicit decision metadata make it suitable for
// embedding in gateways, APIs, and gRPC adapters. Distributed/global quotas
// should still be enforced by an upstream gateway or reviewed shared store.
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*Visitor
	rate     float64
	capacity float64
	cleanupEvery time.Duration
	idleTTL  time.Duration
	maxVisitors int
	stop     chan struct{}
	close    sync.Once
	observer func(client string, decision Decision)
}

// NewRateLimiter creates a limiter with rate tokens refilled per second and
// capacity as the maximum burst size. It keeps the original public API while
// applying safe lifecycle defaults.
func NewRateLimiter(rate, capacity int) *RateLimiter {
	return NewRateLimiterWithConfig(RateLimiterConfig{Rate: rate, Capacity: capacity})
}

// NewRateLimiterWithConfig creates a bounded, lifecycle-safe limiter.
func NewRateLimiterWithConfig(cfg RateLimiterConfig) *RateLimiter {
	if cfg.Rate <= 0 || cfg.Capacity <= 0 {
		panic("rate and capacity must be positive")
	}
	if cfg.CleanupEvery <= 0 {
		cfg.CleanupEvery = time.Minute
	}
	if cfg.IdleTTL <= 0 {
		cfg.IdleTTL = 3 * time.Minute
	}
	if cfg.MaxVisitors <= 0 {
		cfg.MaxVisitors = 100_000
	}

	rl := &RateLimiter{
		visitors:     make(map[string]*Visitor),
		rate:         float64(cfg.Rate),
		capacity:     float64(cfg.Capacity),
		cleanupEvery: cfg.CleanupEvery,
		idleTTL:      cfg.IdleTTL,
		maxVisitors:  cfg.MaxVisitors,
		stop:         make(chan struct{}),
	}
	go rl.cleanup()
	return rl
}

// SetObserver installs an optional telemetry callback. The callback executes
// outside the limiter mutex and must be safe for concurrent use.
func (rl *RateLimiter) SetObserver(observer func(client string, decision Decision)) {
	rl.mu.Lock()
	rl.observer = observer
	rl.mu.Unlock()
}

// Close stops the background cleanup goroutine. It is safe to call repeatedly.
func (rl *RateLimiter) Close() { rl.close.Do(func() { close(rl.stop) }) }

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.cleanupEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			now := time.Now()
			rl.mu.Lock()
			for ip, visitor := range rl.visitors {
				if now.Sub(visitor.lastSeen) > rl.idleTTL {
					delete(rl.visitors, ip)
				}
			}
			rl.mu.Unlock()
		case <-rl.stop:
			return
		}
	}
}

// Allow consumes one token for identity and reports whether the request is allowed.
func (rl *RateLimiter) Allow(identity string) bool {
	return rl.AllowDecision(identity).Allowed
}

// AllowDecision returns admission state plus retry metadata for observability
// and HTTP/gRPC adapters.
func (rl *RateLimiter) AllowDecision(identity string) Decision {
	if identity == "" {
		return Decision{}
	}

	now := time.Now()
	rl.mu.Lock()
	visitor, exists := rl.visitors[identity]
	if !exists {
		if len(rl.visitors) >= rl.maxVisitors {
			decision := Decision{Allowed: false, RetryAfter: time.Second}
			observer := rl.observer
			rl.mu.Unlock()
			if observer != nil {
				observer(identity, decision)
			}
			return decision
		}
		remaining := rl.capacity - 1
		decision := Decision{Allowed: true, Remaining: remaining}
		rl.visitors[identity] = &Visitor{tokens: remaining, lastSeen: now}
		observer := rl.observer
		rl.mu.Unlock()
		if observer != nil {
			observer(identity, decision)
		}
		return decision
	}

	visitor.tokens = min(rl.capacity, visitor.tokens+now.Sub(visitor.lastSeen).Seconds()*rl.rate)
	visitor.lastSeen = now
	if visitor.tokens < 1 {
		missing := 1 - visitor.tokens
		retry := time.Duration(missing/rl.rate*float64(time.Second))
		if retry < time.Millisecond {
			retry = time.Millisecond
		}
		decision := Decision{Allowed: false, Remaining: visitor.tokens, RetryAfter: retry}
		observer := rl.observer
		rl.mu.Unlock()
		if observer != nil {
			observer(identity, decision)
		}
		return decision
	}

	visitor.tokens--
	decision := Decision{Allowed: true, Remaining: visitor.tokens}
	observer := rl.observer
	rl.mu.Unlock()
	if observer != nil {
		observer(identity, decision)
	}
	return decision
}

func min(left, right float64) float64 {
	if left < right {
		return left
	}
	return right
}

func clientIP(remoteAddr string) string {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	return remoteAddr
}

// IdentityExtractor determines the quota identity. The default is the peer IP.
// Deployments behind a trusted gateway can inject a tenant/user extractor
// without making the limiter itself trust spoofable forwarding headers.
type IdentityExtractor func(*http.Request) string

func limitMiddleware(rl *RateLimiter, next http.Handler) http.Handler {
	return limitMiddlewareWithIdentity(rl, clientIPFromRequest)
}

func limitMiddlewareWithIdentity(rl *RateLimiter, identity IdentityExtractor) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			client := identity(r)
			decision := rl.AllowDecision(client)
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(int(rl.capacity)))
			w.Header().Set("X-RateLimit-Remaining", formatRemaining(decision.Remaining))
			if !decision.Allowed {
				seconds := int((decision.RetryAfter + time.Second - 1) / time.Second)
				if seconds < 1 {
					seconds = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(seconds))
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func formatRemaining(remaining float64) string {
	if remaining < 0 {
		return "0"
	}
	return fmt.Sprintf("%.3f", remaining)
}

func clientIPFromRequest(r *http.Request) string {
	return clientIP(r.RemoteAddr)
}

func main() {
	rl := NewRateLimiter(2, 5)
	defer rl.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("OK")) })
	log.Println("Rate limiter running on :8080")
	log.Fatal(http.ListenAndServe(":8080", limitMiddleware(rl, mux)))
}
