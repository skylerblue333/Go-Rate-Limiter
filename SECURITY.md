# Security Policy

## Current security model

Sky Rate Guard is a process-local rate-limiting component. Treat it as one control in a layered architecture, not as a complete DDoS or authentication system.

### Safe deployment expectations

- terminate public TLS at a reviewed ingress or gateway;
- do not expose `/metrics` publicly without an access-control layer;
- if `IDENTITY_HEADER` is configured, only trust a header written by an authenticated upstream proxy or gateway;
- do not trust arbitrary client-supplied `X-Forwarded-For` values;
- keep the service behind network policy when used as a decision sidecar;
- use distributed/shared rate limiting when multiple replicas must enforce one global quota;
- set `MAX_VISITORS` to a value appropriate for available memory and expected tenant cardinality;
- monitor denied/allowed counts and active visitors for abuse patterns.

## Explicit non-goals

The current release does not provide credential authentication, TLS termination, WAF rules, bot classification, distributed consensus, persistent quota storage, or edge-network DDoS absorption.

## Reporting

Do not include secrets, access tokens, or customer data in public vulnerability reports. Provide a minimal reproducer and affected commit SHA.
