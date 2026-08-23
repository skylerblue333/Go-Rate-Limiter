# Sky Rate Guard

Sky Rate Guard is a small, independently deployable HTTP rate-limiting decision service and Go middleware component for APIs, gateways, internal tools, and SKYCOIN4444 services.

## Product boundary

### Implemented

- process-local token-bucket limiting;
- configurable requests-per-second and burst capacity;
- bounded active-client cardinality to reduce memory-exhaustion risk;
- optional explicit identity-header limiting;
- IP-based fallback identity without blindly trusting forwarded headers;
- HTTP middleware returning `429` plus `Retry-After`;
- `POST /v1/check` decision API for gateway/sidecar integration;
- JSON operational counters;
- health/readiness endpoints;
- graceful process shutdown;
- non-root minimal container image;
- Docker Compose packaging;
- test, race, vet, vulnerability-scan, binary-build, and image-build CI gates.

### Not claimed

- globally distributed quotas;
- Redis-backed or database-backed shared state;
- billing/metering or tenant administration UI;
- DDoS mitigation at network edge;
- WAF behavior;
- API-key authentication;
- mTLS/TLS termination;
- durable analytics;
- an availability or throughput SLA.

## Commercial packaging paths

1. **Self-hosted API guard** — deploy the container next to an API or gateway.
2. **Embedded Go middleware** — extract the limiter package into existing Go services.
3. **Decision sidecar** — gateways call `/v1/check` before forwarding a protected operation.
4. **Managed service future** — add durable distributed state, tenant control plane, auth, usage analytics, and billing after those capabilities have their own tests and operational evidence.

## Why this is independently useful

The component has a narrow contract and does not require the rest of SKYCOIN4444. It can protect unrelated APIs while remaining an infrastructure building block inside the larger ecosystem.
