# Go Rate Limiter

Production-oriented, in-process token-bucket HTTP middleware for Go services that need deterministic request admission, burst control, bounded identity memory, lifecycle-safe cleanup, and telemetry integration.

> **SkyCoin4444 / IITR infrastructure component:** reusable traffic-control infrastructure for APIs, gateways, gRPC adapters, internal services, and marketplace workloads.

## Enterprise capability

- Fractional token-bucket refill for smooth admission control.
- Configurable refill rate and burst capacity.
- Bounded visitor state with a configurable `MaxVisitors` safety limit.
- Configurable stale-entry cleanup interval and idle TTL.
- Race-safe concurrent access.
- Idempotent shutdown for service and test lifecycles.
- Structured `Decision` metadata with remaining capacity and retry timing.
- Optional observer callback for Prometheus/OpenTelemetry/application metrics.
- HTTP middleware with `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `429`, and calculated `Retry-After` headers.
- Pluggable identity extraction for trusted tenant/user identities without making the limiter trust spoofable forwarding headers.
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
rl := NewRateLimiterWithConfig(RateLimiterConfig{
    Rate:         100,
    Capacity:     200,
    MaxVisitors:  100_000,
    CleanupEvery: time.Minute,
    IdleTTL:      3 * time.Minute,
})
defer rl.Close()

handler := limitMiddleware(rl, yourHandler)
```

For tenant-aware deployments behind a trusted gateway, provide an explicit identity extractor:

```go
handler := limitMiddlewareWithIdentity(rl, func(r *http.Request) string {
    return r.Header.Get("X-Trusted-Tenant-ID")
})(yourHandler)
```

Only use identity headers that are established by a trusted authenticated boundary. Do not accept arbitrary client-controlled forwarding headers as a security identity.

## Decision and telemetry contract

`AllowDecision` exposes an explicit admission result instead of forcing callers to infer state from a boolean:

```go
decision := rl.AllowDecision(identity)
if !decision.Allowed {
    // decision.RetryAfter contains the estimated delay before another token.
}
```

Attach the observer to feed the service's existing telemetry pipeline. The callback runs outside the limiter lock:

```go
rl.SetObserver(func(client string, decision Decision) {
    // Export allowed/denied counts and remaining capacity.
})
```

## Architecture

```text
                         TRUSTED EDGE / API GATEWAY
                                   |
                         authenticated identity
                                   v
+----------------+       +----------------------+       +------------------+
| HTTP / gRPC    | ----> | Go Rate Limiter     | ----> | Application      |
| clients        |       | token bucket        |       | / protocol       |
+----------------+       +----------+-----------+       +------------------+
                                    |
                         decision + remaining
                                    v
                         +----------------------+
                         | Telemetry Observer   |
                         | metrics / tracing    |
                         +----------------------+

Memory safety boundary:
  MaxVisitors + IdleTTL + CleanupEvery prevent unbounded process-local state.

Distributed quota boundary:
  Replicas are independent. Global quotas belong at a trusted edge or a
  deliberately designed shared-state service.
```

## Marketplace and platform value surfaces

This component can serve as infrastructure for legitimate commercial products. These are **potential revenue streams**, not claims of current revenue or valuation:

1. Embeddable API-rate-limit middleware licensing/support.
2. Managed API quota service.
3. Multi-tenant SaaS usage-control layer.
4. Gateway integration package for Go microservice platforms.
5. gRPC service quota adapter.
6. Observability and telemetry integration package.
7. Enterprise deployment and architecture services.
8. Security hardening and abuse-control assessments.
9. SDKs/connectors for SkyCoin4444 ecosystem services.
10. Premium support, SLA, and maintenance contracts.
11. Training, implementation, and migration services.

## Production boundary

This repository intentionally remains process-local. It should not be presented as a globally consistent quota system. For institutional deployments, pair it with:

- authenticated tenant/user identity;
- an upstream gateway or service mesh;
- distributed quota state where global enforcement is required;
- metrics, tracing, dashboards, and alerts;
- explicit fail-open/fail-closed policy;
- load and chaos testing in a staging environment;
- signed release artifacts and dependency/security scanning.

## Verification

GitHub Actions is configured to verify formatting, `go vet`, race-detector tests, benchmark smoke tests, and Go vulnerability analysis on pushes and pull requests.

The repository's engineering claims should be treated as verified only when the corresponding GitHub Actions run is green. Performance numbers must come from recorded benchmark output rather than marketing estimates.

## Open-source provenance

The implementation favors Go's standard library and explicit interfaces rather than copying large external codebases. Proven patterns are adopted at the design/API level while preserving clear licensing and maintainability boundaries.

## License

See [`LICENSE`](LICENSE).
