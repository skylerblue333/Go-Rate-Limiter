# Architecture — Go Rate Limiter

## Purpose

Provide deterministic, low-overhead request admission control for a single Go process while exposing production-grade seams for telemetry, trusted identity extraction, bounded memory, and HTTP response metadata.

## State model

Each client or tenant identity owns one token bucket:

- `capacity`: maximum stored tokens / burst allowance.
- `rate`: refill tokens per second.
- `tokens`: fractional current balance.
- `lastSeen`: timestamp used to calculate refill.

For elapsed interval `dt`:

`tokens' = min(capacity, tokens + dt * rate)`

An accepted request consumes one token. A request is rejected when the resulting balance is below one token.

## Admission decision

The limiter returns a `Decision` containing:

- `Allowed` — whether the request can proceed.
- `Remaining` — fractional token balance after the decision.
- `RetryAfter` — estimated wait time when admission is denied.

This makes the control plane explicit for HTTP, gRPC, metrics, and future gateway adapters.

## Memory and lifecycle safety

The visitor map is bounded by `MaxVisitors`. Idle entries are removed using configurable `CleanupEvery` and `IdleTTL` values. When the visitor bound is reached, new identities are denied rather than allowing attacker-controlled identity cardinality to grow process memory without limit.

`Close()` stops the cleanup goroutine through `sync.Once`, making shutdown idempotent for services, tests, and graceful termination paths.

## Concurrency

All bucket-map mutation is protected by a mutex. Observer callbacks execute after the mutex is released, preventing application telemetry from blocking limiter state mutation or creating callback-induced lock cycles.

## Request path

```text
+---------+       +----------------------+       +-------------------+
| Client  | ----> | Trusted Edge / HTTP  | ----> | Identity Extractor|
+---------+       | Middleware           |       +---------+---------+
                  +----------------------+                 |
                                                           v
                                                +-------------------+
                                                | RateLimiter       |
                                                | token bucket      |
                                                +---------+---------+
                                                          |
                              +---------------------------+-------------------+
                              |                                               |
                           allowed                                         denied
                              |                                               |
                              v                                               v
                       +-------------+                               +----------------+
                       | Next Handler|                               | HTTP 429        |
                       +------+------+                               | Retry-After    |
                              |                                      | RateLimit-*    |
                              v                                      +----------------+
                       application
                              |
                              v
                     observer / telemetry
                              |
                              v
                   metrics / tracing / logs
```

## Security boundaries

The limiter does not automatically trust `X-Forwarded-For`, `Forwarded`, or arbitrary client-controlled headers. Deployments behind a reverse proxy should establish identity at a trusted authenticated boundary and pass that identity through the explicit `IdentityExtractor` interface.

This component is a traffic-control primitive, not an authentication system, WAF, DDoS mitigation service, or distributed consensus layer.

## Scaling boundary

State is process-local. Horizontal replicas therefore have independent buckets. This is a deliberate boundary. A global quota layer belongs above this component and should define:

1. identity and authorization;
2. consistency requirements;
3. failure behavior;
4. abuse policy;
5. persistence and retention;
6. observability and reconciliation.

## Institutional integration pattern

```text
Internet
   |
   v
WAF / DDoS / Edge Gateway
   |
   +--> authenticated tenant identity
   |
   v
Go Rate Limiter  --->  gRPC / HTTP service
   |                         |
   +--> telemetry ------------+--> application metrics
   |
   +--> local bounded state

Global quota / billing policy remains upstream.
```

## Verification contract

Every change is expected to pass:

- `gofmt` check
- `go vet ./...`
- `go test -race ./...`
- benchmark smoke test
- `govulncheck ./...`

GitHub Actions is the authoritative repository verification path. A performance claim should only be published after a reproducible benchmark run records the actual environment and output.
