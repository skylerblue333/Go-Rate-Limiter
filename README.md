# Go Rate Limiter

Production-oriented, in-process token-bucket HTTP middleware for Go services that need deterministic request admission, burst control, lifecycle-safe cleanup, and an integration point for telemetry.

> **SkyCoin4444 / IITR infrastructure component:** designed as a reusable traffic-control primitive that can sit in front of APIs, gateways, gRPC adapters, and internal services.

## What is implemented

- Fractional token-bucket refill for smooth admission control.
- Configurable refill rate and burst capacity.
- Process-local client state with automatic stale-entry cleanup.
- Race-safe concurrent access.
- Idempotent shutdown for service and test lifecycles.
- Optional observer callback for Prometheus/OpenTelemetry/application metrics.
- HTTP middleware returning `429 Too Many Requests` with `Retry-After: 1` on denial.
- IPv4/IPv6-safe `host:port` extraction through `net.SplitHostPort`.
- Unit tests, race tests, `go vet`, formatting verification, benchmark coverage, and `govulncheck` in GitHub Actions.

## Quick start

```bash
go test -race ./...
go vet ./...
go test -run '^$' -bench BenchmarkRateLimiterAllow -benchtime=1s ./...
go run .
```

The example server listens on `:8080`.

## Embedding

```go
rl := NewRateLimiter(100, 200)
defer rl.Close()

handler := limitMiddleware(rl, yourHandler)
```

For production telemetry, attach an observer and forward the event to the metrics system already used by your service:

```go
rl.SetObserver(func(client string, allowed bool, remaining float64) {
    // Export to your existing metrics/telemetry pipeline.
})
```

## Architecture

```text
Client
  |
  v
HTTP / API Gateway
  |
  v
RateLimiter.Allow(identity)
  |---- denied ----> HTTP 429 + Retry-After
  |
  +---- allowed ---> Application / gRPC / protocol service
                         |
                         +--> telemetry observer
```

The limiter deliberately does **not** pretend to be a distributed quota service. Replicas maintain independent buckets. Global quotas, tenant-wide limits, and abuse controls should be enforced at an upstream gateway or with a reviewed shared-state design.

## Productization / value surfaces

This repository can support multiple legitimate commercial offerings without pretending that revenue already exists:

1. Embeddable API-rate-limit middleware licensing/support.
2. Managed API quota service built around the same admission-control contract.
3. Multi-tenant SaaS usage-control layer.
4. Gateway integration package for Go microservice platforms.
5. gRPC service quota adapter.
6. Observability/telemetry integration package.
7. Enterprise deployment and architecture services.
8. Security hardening and abuse-control assessments.
9. SDKs/connectors for SkyCoin4444 ecosystem services.
10. Premium support, SLA, and maintenance contracts.
11. Training, implementation, and migration services.

These are **potential revenue streams**, not claims of current revenue, valuation, or customer adoption.

## Verification

GitHub Actions verifies formatting, `go vet`, race-detector tests, a benchmark smoke test, and Go vulnerability analysis on pushes and pull requests.

## Scope

This component is intentionally process-local. A production architecture requiring globally consistent quotas should pair it with a trusted edge gateway, tenant identity model, distributed state where required, metrics, alerting, and an explicit failure policy.
