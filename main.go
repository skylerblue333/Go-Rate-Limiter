package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	defaultListenAddr  = ":8080"
	defaultRate        = 10
	defaultBurst       = 20
	defaultMaxVisitors = 100000
	visitorTTL         = 3 * time.Minute
	cleanupInterval    = time.Minute
)

type Visitor struct {
	tokens   float64
	lastSeen time.Time
}

type Decision struct {
	Allowed      bool    `json:"allowed"`
	Remaining    float64 `json:"remaining"`
	RetryAfterMS int64   `json:"retry_after_ms,omitempty"`
}

type LimiterStats struct {
	Allowed        uint64 `json:"allowed"`
	Denied         uint64 `json:"denied"`
	ActiveVisitors int    `json:"active_visitors"`
	MaxVisitors    int    `json:"max_visitors"`
}

type RateLimiter struct {
	mu          sync.Mutex
	visitors    map[string]*Visitor
	rate        float64
	capacity    float64
	maxVisitors int
	stop        chan struct{}
	close       sync.Once
	observer    func(client string, allowed bool, remaining float64)
	allowed     atomic.Uint64
	denied      atomic.Uint64
}

func NewRateLimiter(rate, capacity int) *RateLimiter {
	return NewRateLimiterWithLimit(rate, capacity, defaultMaxVisitors)
}

func NewRateLimiterWithLimit(rate, capacity, maxVisitors int) *RateLimiter {
	if rate <= 0 || capacity <= 0 || maxVisitors <= 0 {
		panic("rate, capacity, and maxVisitors must be positive")
	}
	rl := &RateLimiter{
		visitors:    make(map[string]*Visitor),
		rate:        float64(rate),
		capacity:    float64(capacity),
		maxVisitors: maxVisitors,
		stop:        make(chan struct{}),
	}
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) SetObserver(observer func(client string, allowed bool, remaining float64)) {
	rl.mu.Lock()
	rl.observer = observer
	rl.mu.Unlock()
}

func (rl *RateLimiter) Close() { rl.close.Do(func() { close(rl.stop) }) }

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case now := <-ticker.C:
			rl.mu.Lock()
			for key, visitor := range rl.visitors {
				if now.Sub(visitor.lastSeen) > visitorTTL {
					delete(rl.visitors, key)
				}
			}
			rl.mu.Unlock()
		case <-rl.stop:
			return
		}
	}
}

func (rl *RateLimiter) Allow(key string) bool { return rl.Take(key).Allowed }

func (rl *RateLimiter) Take(key string) Decision {
	key = strings.TrimSpace(key)
	if key == "" {
		rl.denied.Add(1)
		return Decision{Allowed: false, RetryAfterMS: 1000}
	}

	now := time.Now()
	rl.mu.Lock()
	visitor, exists := rl.visitors[key]
	if !exists {
		if len(rl.visitors) >= rl.maxVisitors {
			observer := rl.observer
			rl.mu.Unlock()
			rl.denied.Add(1)
			if observer != nil {
				observer(key, false, 0)
			}
			return Decision{Allowed: false, Remaining: 0, RetryAfterMS: 1000}
		}
		remaining := rl.capacity - 1
		rl.visitors[key] = &Visitor{tokens: remaining, lastSeen: now}
		observer := rl.observer
		rl.mu.Unlock()
		rl.allowed.Add(1)
		if observer != nil {
			observer(key, true, remaining)
		}
		return Decision{Allowed: true, Remaining: remaining}
	}

	visitor.tokens = min(rl.capacity, visitor.tokens+now.Sub(visitor.lastSeen).Seconds()*rl.rate)
	visitor.lastSeen = now
	if visitor.tokens < 1 {
		remaining := visitor.tokens
		retryAfter := int64(math.Ceil(((1 - remaining) / rl.rate) * 1000))
		if retryAfter < 1 {
			retryAfter = 1
		}
		observer := rl.observer
		rl.mu.Unlock()
		rl.denied.Add(1)
		if observer != nil {
			observer(key, false, remaining)
		}
		return Decision{Allowed: false, Remaining: remaining, RetryAfterMS: retryAfter}
	}

	visitor.tokens--
	remaining := visitor.tokens
	observer := rl.observer
	rl.mu.Unlock()
	rl.allowed.Add(1)
	if observer != nil {
		observer(key, true, remaining)
	}
	return Decision{Allowed: true, Remaining: remaining}
}

func (rl *RateLimiter) Stats() LimiterStats {
	rl.mu.Lock()
	active := len(rl.visitors)
	maxVisitors := rl.maxVisitors
	rl.mu.Unlock()
	return LimiterStats{
		Allowed:        rl.allowed.Load(),
		Denied:         rl.denied.Load(),
		ActiveVisitors: active,
		MaxVisitors:    maxVisitors,
	}
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

func requestIdentity(r *http.Request, identityHeader string) (string, error) {
	if identityHeader == "" {
		identity := strings.TrimSpace(clientIP(r.RemoteAddr))
		if identity == "" {
			return "", errors.New("client address is unavailable")
		}
		return identity, nil
	}
	identity := strings.TrimSpace(r.Header.Get(identityHeader))
	if identity == "" {
		return "", fmt.Errorf("missing required identity header %s", identityHeader)
	}
	if len(identity) > 256 {
		return "", errors.New("identity is too long")
	}
	return identity, nil
}

func limitMiddleware(rl *RateLimiter, identityHeader string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, err := requestIdentity(r, identityHeader)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		decision := rl.Take(identity)
		w.Header().Set("X-RateLimit-Remaining", strconv.FormatFloat(decision.Remaining, 'f', 2, 64))
		if !decision.Allowed {
			retrySeconds := int64(math.Ceil(float64(decision.RetryAfterMS) / 1000))
			if retrySeconds < 1 {
				retrySeconds = 1
			}
			w.Header().Set("Retry-After", strconv.FormatInt(retrySeconds, 10))
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type checkRequest struct {
	Key string `json:"key"`
}

func checkHandler(rl *RateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		defer r.Body.Close()
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		var input checkRequest
		if err := decoder.Decode(&input); err != nil || strings.TrimSpace(input.Key) == "" {
			http.Error(w, "invalid decision request", http.StatusBadRequest)
			return
		}
		if len(input.Key) > 256 {
			http.Error(w, "key is too long", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rl.Take(input.Key))
	}
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "sky-rate-guard"})
}

func metricsHandler(rl *RateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rl.Stats())
	}
}

type config struct {
	Addr           string
	Rate           int
	Burst          int
	MaxVisitors    int
	IdentityHeader string
}

func envInt(name string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return value, nil
}

func loadConfig() (config, error) {
	cfg := config{Addr: strings.TrimSpace(os.Getenv("LISTEN_ADDR")), IdentityHeader: strings.TrimSpace(os.Getenv("IDENTITY_HEADER"))}
	if cfg.Addr == "" {
		cfg.Addr = defaultListenAddr
	}
	var err error
	if cfg.Rate, err = envInt("RATE_LIMIT_RPS", defaultRate); err != nil {
		return config{}, err
	}
	if cfg.Burst, err = envInt("RATE_LIMIT_BURST", defaultBurst); err != nil {
		return config{}, err
	}
	if cfg.MaxVisitors, err = envInt("MAX_VISITORS", defaultMaxVisitors); err != nil {
		return config{}, err
	}
	return cfg, nil
}

func runHealthcheck(addr string) error {
	url := addr
	if strings.HasPrefix(url, ":") {
		url = "127.0.0.1" + url
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "http://" + url
	}
	client := http.Client{Timeout: 2 * time.Second}
	response, err := client.Get(url + "/healthz")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck returned %s", response.Status)
	}
	return nil
}

func main() {
	healthcheck := flag.Bool("healthcheck", false, "check the local service health endpoint and exit")
	flag.Parse()
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}
	if *healthcheck {
		if err := runHealthcheck(cfg.Addr); err != nil {
			log.Fatal(err)
		}
		return
	}

	rl := NewRateLimiterWithLimit(cfg.Rate, cfg.Burst, cfg.MaxVisitors)
	defer rl.Close()

	protected := http.NewServeMux()
	protected.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("request allowed by Sky Rate Guard\n")) })

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthHandler)
	mux.HandleFunc("/readyz", healthHandler)
	mux.HandleFunc("/metrics", metricsHandler(rl))
	mux.HandleFunc("/v1/check", checkHandler(rl))
	mux.Handle("/", limitMiddleware(rl, cfg.IdentityHeader, protected))

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("Sky Rate Guard listening on %s (rate=%d/s burst=%d max_visitors=%d identity_header=%q)", cfg.Addr, cfg.Rate, cfg.Burst, cfg.MaxVisitors, cfg.IdentityHeader)
		errCh <- server.ListenAndServe()
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-signals:
		log.Printf("received %s; shutting down", sig)
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}
