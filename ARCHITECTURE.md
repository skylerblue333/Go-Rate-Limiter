# Architecture — Go Rate Limiter

## Purpose

Provide deterministic, low-overhead request admission control for a single Go process while exposing a safe integration seam for production telemetry.

## State model

Each client identity owns one token bucket:

- `capacity`: maximum stored tokens / burst allowance.
- `rate`: refill tokens per second.
- `tokens`: fractional current balance.
- `lastSeen`: timestamp used to calculate refill.

For an elapsed interval `dt`, the refill is:

`tokens' = min(capacity, tokens + dt * rate)`

An accepted request consumes one token. A request is rejected when the resulting available balance is below one token.

## Concurrency

All bucket-map mutation is protected by a single mutex. Observer callbacks execute after the mutex is released, preventing application telemetry from blocking limiter state mutation or creating callback-induced lock cycles.

## Lifecycle

A background cleanup goroutine runs once per minute and removes identities that have been idle for more than three minutes. `Close()` closes the stop channel through `sync.Once`, making shutdown safe for repeated service cleanup and tests.

## Request path

```text
+---------+       +----------------+       +-------------------+
| Client  | ----> | HTTP Middleware| ----> | RateLimiter.Allow  |
+---------+       +----------------+       +---------+---------+
                                                     |
                              +----------------------+-------------------+
                              |                                          |
                           allowed                                    denied
                              |                                          |
                              v                                          v
                       +-------------+                         +----------------+
                       | Next Handler|                         | HTTP 429        |
                       +------+------+                         | Retry-After: 1 |
                              |                                +----------------+
                              v
                       application
                              |
                              v
                       observer hook
                              |
                              v
                  metrics / tracing / logs
```

## Security boundaries

The limiter treats the supplied client identity as authoritative input from the caller. It does not trust `X-Forwarded-For` or other proxy headers automatically. Deployments behind a reverse proxy should establish a documented, trusted identity-extraction policy at the gateway boundary.

## Scaling boundary

State is process-local. Horizontal replicas therefore have independent buckets. This is a deliberate boundary, not an accidental limitation. A global quota layer belongs above this component and should define consistency, identity, failure, and abuse policies explicitly.

## Verification contract

Every change is expected to pass:

- `gofmt` check
- `go vet ./...`
- `go test -race ./...`
- benchmark smoke test
- `govulncheck ./...`

GitHub Actions is the authoritative repository verification path.
