package main

import (
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// Visitor stores the fractional token-bucket state for one client identity.
type Visitor struct {
	tokens   float64
	lastSeen time.Time
}

// RateLimiter is a process-local token-bucket limiter.
//
// The implementation is intentionally deterministic and dependency-free so it
// can be embedded in gateways, APIs, and internal services. Distributed/global
// quotas should be enforced by an upstream gateway or shared store.
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*Visitor
	rate     float64
	capacity float64
	stop     chan struct{}
	close    sync.Once
	observer func(client string, allowed bool, remaining float64)
}

// NewRateLimiter creates a limiter with rate tokens refilled per second and
// capacity as the maximum burst size. Invalid configuration panics early so a
// service cannot silently start with an unsafe limiter.
func NewRateLimiter(rate, capacity int) *RateLimiter {
	if rate <= 0 || capacity <= 0 {
		panic("rate and capacity must be positive")
	}
	rl := &RateLimiter{
		visitors: make(map[string]*Visitor),
		rate:     float64(rate),
		capacity: float64(capacity),
		stop:     make(chan struct{}),
	}
	go rl.cleanup()
	return rl
}

// SetObserver installs an optional telemetry callback. The callback executes
// outside the limiter mutex and must be safe for concurrent use.
func (rl *RateLimiter) SetObserver(observer func(client string, allowed bool, remaining float64)) {
	rl.mu.Lock()
	rl.observer = observer
	rl.mu.Unlock()
}

// Close stops the background cleanup goroutine. It is safe to call repeatedly.
func (rl *RateLimiter) Close() { rl.close.Do(func() { close(rl.stop) }) }

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			now := time.Now()
			rl.mu.Lock()
			for ip, visitor := range rl.visitors {
				if now.Sub(visitor.lastSeen) > 3*time.Minute {
					delete(rl.visitors, ip)
				}
			}
			rl.mu.Unlock()
		case <-rl.stop:
			return
		}
	}
}

// Allow consumes one token for ip and reports whether the request is allowed.
func (rl *RateLimiter) Allow(ip string) bool {
	if ip == "" {
		return false
	}

	now := time.Now()
	rl.mu.Lock()
	visitor, exists := rl.visitors[ip]
	if !exists {
		remaining := rl.capacity - 1
		rl.visitors[ip] = &Visitor{tokens: remaining, lastSeen: now}
		observer := rl.observer
		rl.mu.Unlock()
		if observer != nil {
			observer(ip, true, remaining)
		}
		return true
	}

	visitor.tokens = min(rl.capacity, visitor.tokens+now.Sub(visitor.lastSeen).Seconds()*rl.rate)
	visitor.lastSeen = now
	if visitor.tokens < 1 {
		remaining := visitor.tokens
		observer := rl.observer
		rl.mu.Unlock()
		if observer != nil {
			observer(ip, false, remaining)
		}
		return false
	}

	visitor.tokens--
	remaining := visitor.tokens
	observer := rl.observer
	rl.mu.Unlock()
	if observer != nil {
		observer(ip, true, remaining)
	}
	return true
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

func limitMiddleware(rl *RateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client := clientIP(r.RemoteAddr)
		if !rl.Allow(client) {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	rl := NewRateLimiter(2, 5)
	defer rl.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("OK")) })
	log.Println("Rate limiter running on :8080")
	log.Fatal(http.ListenAndServe(":8080", limitMiddleware(rl, mux)))
}
