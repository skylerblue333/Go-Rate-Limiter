# Sky Rate Guard

A focused, independently deployable token-bucket rate limiter for APIs and gateways, built in Go and packaged as a small container.

Sky Rate Guard can run in two modes at the same time:

- **middleware guard** — the root route is protected and returns `429 Too Many Requests` when a client exceeds quota;
- **decision service** — `POST /v1/check` accepts a client/tenant key and returns an allow/deny decision for an upstream gateway.

## Quick start

```bash
RATE_LIMIT_RPS=10 \
RATE_LIMIT_BURST=20 \
MAX_VISITORS=100000 \
go run .
```

Health and metrics:

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/metrics
```

Decision API:

```bash
curl -sS http://localhost:8080/v1/check \
  -H 'content-type: application/json' \
  -d '{"key":"tenant-123"}'
```

Example response:

```json
{"allowed":true,"remaining":19}
```

## Configuration

| Variable | Default | Purpose |
|---|---:|---|
| `LISTEN_ADDR` | `:8080` | HTTP listen address |
| `RATE_LIMIT_RPS` | `10` | tokens refilled per second |
| `RATE_LIMIT_BURST` | `20` | maximum token capacity |
| `MAX_VISITORS` | `100000` | hard bound on process-local identities |
| `IDENTITY_HEADER` | empty | optional trusted identity header for middleware mode |

When `IDENTITY_HEADER` is empty, middleware mode uses the TCP peer address. The service intentionally does **not** trust `X-Forwarded-For` automatically because that header can be client-spoofed unless a trusted proxy sanitizes it.

## Runtime endpoints

- `GET /healthz` — liveness
- `GET /readyz` — readiness
- `GET /metrics` — compact JSON decision counters/cardinality
- `POST /v1/check` — sidecar/gateway allow-deny decision API
- `/` — example middleware-protected route

Denied middleware requests include `Retry-After` and `X-RateLimit-Remaining` headers.

## Container deployment

```bash
docker compose up --build
```

The image runs as a non-root user on a distroless runtime. The Compose example additionally uses a read-only filesystem and `no-new-privileges`.

## Verification

```bash
gofmt -w *.go
go vet ./...
go test ./...
go test -race ./...
go test -run '^$' -bench BenchmarkRateLimiterAllow -benchtime=100ms ./...
```

GitHub Actions additionally runs `govulncheck`, compiles the standalone binary, and builds the production container image.

## Product status

### Implemented and testable

- token-bucket refill/burst behavior;
- per-identity isolation;
- bounded active identity map;
- middleware and decision-service integration models;
- configurable identity header;
- operational decision counters;
- health/readiness probes;
- graceful shutdown;
- Docker deployment package.

### Deliberately not claimed

Sky Rate Guard currently provides **process-local** quotas. It is not yet a globally distributed rate-limit service and does not claim Redis-backed shared quotas, mTLS, edge DDoS absorption, WAF functionality, billing, a tenant control plane, or an SLA.

See [`PRODUCT.md`](PRODUCT.md) for commercial packaging boundaries and [`SECURITY.md`](SECURITY.md) before exposing the service to public traffic.

## SKYCOIN4444 integration

Sky Rate Guard is productized independently so it can be deployed or sold by itself, while remaining reusable as the rate-limiting boundary in the wider SKYCOIN4444 gateway and microservice architecture.
