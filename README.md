# Go Rate Limiter

A small in-process token-bucket HTTP middleware for controlled services. It is not a distributed rate-limit service and does not claim enterprise-scale capacity.

## Implemented behavior

The limiter supports a configurable refill rate and burst capacity, uses fractional token accounting for smoother refill behavior, extracts the client host from `host:port`, returns HTTP 429 when the bucket is empty, validates positive configuration, and exposes a stoppable cleanup lifecycle so tests and services can shut it down cleanly.

## Validation

```bash
go test -race ./...
go vet ./...
```

The current suite passes with the race detector and `go vet`. Tests cover burst behavior, refill, invalid configuration, cleanup lifecycle, and client-IP parsing.

## Scope and limitations

State is process-local and is not shared across replicas. A production deployment requiring global quotas should use a reviewed distributed store or gateway-level limiter, trusted proxy configuration, metrics, and operational policy. The former “enterprise-grade,” “scalable,” and “cloud-native” claims were removed because the implementation does not substantiate them.
