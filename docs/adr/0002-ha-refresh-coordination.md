# ADR 0002: Defer high-availability refresh coordination

## Status

Accepted for v0.2 and v0.3; reaffirmed for v1.0.

## Context

Multiple replicas independently call billable AWS APIs, maintain unrelated in-memory snapshots, and expose duplicate Prometheus targets. v0.2 adds up to 20 targets, making uncoordinated duplication more expensive.

## Decision

v1 keeps `replicaCount: 1` and does not implement leader election or shared
persistence. Helm uses a cost-first non-overlapping RollingUpdate with
`maxSurge: 0` and `maxUnavailable: 1`. The old pod must stop before the new pod
starts, preventing a duplicate paid-request window at the cost of a temporary metrics outage during upgrade.

Options evaluated:

- Kubernetes Lease: simple in Kubernetes, but adds platform coupling and failover/staleness semantics.
- External shared cache: permits active/active serving, but introduces another production dependency and serialization contract.
- Duplicate refreshes: simplest, but multiplies AWS cost and can produce divergent readiness.
- Static target sharding: bounds duplication, but requires external ownership and rebalance procedures.

## Consequences

Operators receive a predictable single-writer, memory-only cache. Availability
during pod replacement relies on Kubernetes restart behavior and Prometheus
retention. Rollback uses the same non-overlapping replacement. A later release
must define ownership, fencing, stale-leader behavior, and request-cost tests
before enabling multiple replicas.
