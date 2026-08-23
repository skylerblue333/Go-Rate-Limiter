# Institutional Integration Contract

## Role

`Go-Rate-Limiter` is the reusable admission-control primitive for API gateways, gRPC services, internal workloads, and tenant quotas.

## Integration sequence

`client identity -> Py-Microservice-Gateway -> Go-Rate-Limiter -> Go-gRPC-Service -> domain service`

`Rust-Circuit-Breaker` should sit around failure-prone upstream calls; telemetry should consume the limiter observer without coupling the limiter to a specific vendor.

## Production gates

The current component is intentionally process-local. A production global-quota design must add a trusted identity model, distributed state where consistency is required, abuse policy, metrics, and explicit fail-open/fail-closed behavior. The existing repository already verifies formatting, vet, race tests, benchmark smoke testing, and Go vulnerability analysis in GitHub Actions.

Do not represent process-local buckets as globally consistent quotas.
