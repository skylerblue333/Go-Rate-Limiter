package main

import (
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

type Visitor struct {
	tokens   float64
	lastSeen time.Time
}

type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*Visitor
	rate     float64
	capacity float64
	stop     chan struct{}
	close    sync.Once
}

func NewRateLimiter(rate, capacity int) *RateLimiter {
	if rate <= 0 || capacity <= 0 {
		panic("rate and capacity must be positive")
	}
	rl := &RateLimiter{visitors: make(map[string]*Visitor), rate: float64(rate), capacity: float64(capacity), stop: make(chan struct{})}
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) Close() { rl.close.Do(func() { close(rl.stop) }) }

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			for ip, visitor := range rl.visitors {
				if time.Since(visitor.lastSeen) > 3*time.Minute {
					delete(rl.visitors, ip)
				}
			}
			rl.mu.Unlock()
		case <-rl.stop:
			return
		}
	}
}

func (rl *RateLimiter) Allow(ip string) bool {
	if ip == "" {
		return false
	}
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()
	visitor, exists := rl.visitors[ip]
	if !exists {
		rl.visitors[ip] = &Visitor{tokens: rl.capacity - 1, lastSeen: now}
		return true
	}
	visitor.tokens = min(rl.capacity, visitor.tokens+now.Sub(visitor.lastSeen).Seconds()*rl.rate)
	visitor.lastSeen = now
	if visitor.tokens < 1 {
		return false
	}
	visitor.tokens--
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
		if !rl.Allow(clientIP(r.RemoteAddr)) {
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
