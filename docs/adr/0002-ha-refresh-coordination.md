# ADR 0002: Defer high-availability refresh coordination

## Status

Accepted for v0.2 and reaffirmed for v0.3.

## Context

Multiple replicas independently call billable AWS APIs, maintain unrelated in-memory snapshots, and expose duplicate Prometheus targets. v0.2 adds up to 20 targets, making uncoordinated duplication more expensive.

## Decision

v0.2 and v0.3 keep `replicaCount: 1` and do not implement leader election or shared persistence.

Options evaluated:

- Kubernetes Lease: simple in Kubernetes, but adds platform coupling and failover/staleness semantics.
- External shared cache: permits active/active serving, but introduces another production dependency and serialization contract.
- Duplicate refreshes: simplest, but multiplies AWS cost and can produce divergent readiness.
- Static target sharding: bounds duplication, but requires external ownership and rebalance procedures.

## Consequences

Operators receive a predictable single-writer cache. Availability during pod replacement relies on Kubernetes restart behavior and Prometheus retention. A later release must define ownership, fencing, stale-leader behavior, and request-cost tests before enabling multiple replicas.
