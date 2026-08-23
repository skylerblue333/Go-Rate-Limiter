# Sky Rate Guard Deployment Runbook

## 1. Pre-deployment gate

Use the exact reviewed commit SHA. Before promotion, require GitHub CI to pass formatting, vet, unit tests, race tests, benchmark smoke, vulnerability scanning, binary compilation, and container build.

## 2. Configuration

Set explicit values rather than relying on defaults for a real environment:

```bash
LISTEN_ADDR=:8080
RATE_LIMIT_RPS=50
RATE_LIMIT_BURST=100
MAX_VISITORS=250000
# Optional only behind an authenticated/sanitizing gateway:
# IDENTITY_HEADER=X-API-Key
```

`IDENTITY_HEADER` is an identity-selection mechanism, not authentication. A public client must not be allowed to choose an arbitrary trusted identity header unless an upstream gateway authenticates and rewrites it.

## 3. Container deployment

```bash
docker compose build
docker compose up -d
docker compose ps
```

The sample image runs as non-root and the Compose file uses a read-only filesystem and `no-new-privileges`.

## 4. Smoke verification

```bash
curl -fsS http://127.0.0.1:8080/healthz
curl -fsS http://127.0.0.1:8080/readyz
curl -fsS http://127.0.0.1:8080/metrics
curl -fsS http://127.0.0.1:8080/v1/check \
  -H 'content-type: application/json' \
  -d '{"key":"deployment-smoke"}'
```

Verify repeated requests eventually produce deny decisions according to the configured burst/refill policy.

## 5. Production topology

Recommended boundary:

```text
Internet
   |
TLS/WAF/Auth Gateway
   |
Sky Rate Guard (private network)
   |
Protected API / service
```

Do not publicly expose operational `/metrics` without access controls.

## 6. Scaling rule

The current limiter is process-local. Horizontal replicas each maintain independent buckets. If a customer requires one quota shared across replicas/regions, add a reviewed distributed-state backend and test its atomicity/failure behavior before describing the product as globally distributed.

## 7. Rollback

Keep the previously verified image digest/commit available. Roll back if health checks fail, denial rate deviates unexpectedly, memory cardinality approaches `MAX_VISITORS`, or upstream services observe incorrect quota behavior.

## 8. Evidence to retain

Record the deployed commit SHA, image digest, configuration values excluding secrets, CI run URL, smoke-test outcome, deployment timestamp, and rollback target.
