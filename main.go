package main

import (
	"log"
	"net/http"
	"sync"
	"time"
)

type Visitor struct {
	tokens    int
	lastSeen  time.Time
}

type RateLimiter struct {
	mu         sync.Mutex
	visitors   map[string]*Visitor
	rate       int
	capacity   int
}

func NewRateLimiter(rate, capacity int) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*Visitor),
		rate:     rate,
		capacity: capacity,
	}
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) cleanup() {
	for {
		time.Sleep(time.Minute)
		rl.mu.Lock()
		for ip, v := range rl.visitors {
			if time.Since(v.lastSeen) > 3*time.Minute {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[ip]
	if !exists {
		rl.visitors[ip] = &Visitor{
			tokens:   rl.capacity - 1,
			lastSeen: time.Now(),
		}
		return true
	}

	now := time.Now()
	elapsed := now.Sub(v.lastSeen).Seconds()
	v.tokens += int(elapsed) * rl.rate
	if v.tokens > rl.capacity {
		v.tokens = rl.capacity
	}
	v.lastSeen = now

	if v.tokens > 0 {
		v.tokens--
		return true
	}
	return false
}

func limitMiddleware(rl *RateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if !rl.Allow(ip) {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	rl := NewRateLimiter(2, 5) // 2 req/sec, burst of 5
	
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	log.Println("Rate limiter running on :8080")
	http.ListenAndServe(":8080", limitMiddleware(rl, mux))
}
