# ADR 0001: Asynchronous Snapshot Architecture

- Status: Accepted
- Date: 2026-07-10
- Decision owners: aws-cost-exporter maintainers

## Context

Prometheus commonly scrapes exporters every 15 to 60 seconds. AWS Cost Explorer
data updates only several times per day, API responses may be paginated, calls
can be throttled, and each paginated request can incur cost.

Calling AWS during every scrape would couple Prometheus availability to AWS
latency and credentials. It would also multiply API calls across Prometheus
servers and exporter replicas. A partial AWS response must not become a
partially updated metric set.

The project also needs independently extensible collectors without turning
deployment into a distributed system or depending on Go plugin ABI stability.

## Decision

The exporter will run as a modular monolith with four runtime stages:

1. A scheduler invokes compile-time registered collectors.
2. Collectors query AWS through narrow ports and build domain snapshots.
3. A complete validated snapshot atomically replaces the previous snapshot.
4. The Prometheus adapter serves the current snapshot without calling AWS.

Snapshots are immutable after publication. Failed refreshes update health
metadata but retain the previous successful values. Readiness reflects snapshot
availability and freshness; liveness does not depend on AWS.

Collector registration uses explicit factories in the composition root. The
project will not use package initialization side effects or Go runtime plugins.
Manual constructor injection will be used instead of a dependency injection
framework.

## Consequences

- Scrape latency and reliability are independent of AWS API latency.
- API usage is predictable and governed by schedule, pagination, and rate limits.
- Readers cannot observe half-published data.
- Collectors can be tested with fake ports and changed independently.
- A single binary remains straightforward to operate and secure.

- Fresh data is delayed until the next successful refresh.
- Restarting loses the in-memory snapshot and temporarily makes the instance
  unready.
- Multiple replicas independently query AWS unless a future coordination
  mechanism is enabled.
- Health metrics and stale-data alerts are required to prevent old values from
  being interpreted as current.

## Alternatives considered

### Query AWS during each scrape
Rejected because it creates unbounded duplicate calls, exposes AWS failures to
Prometheus, and makes scrape duration depend on pagination and retries.

### Persist snapshots to a local database
Rejected for the MVP because Prometheus already owns time-series retention.
Persistence adds migration, corruption, encryption, and high-availability
concerns without improving the primary scrape contract.

### Split collectors into independent services
Rejected because the initial collectors share credentials, scheduling, caching,
and telemetry. Separate services would increase operational complexity before
independent scaling is demonstrated.

### Load third-party Go plugins dynamically
Rejected because Go plugins have platform and toolchain constraints, complicate
supply-chain verification, and do not provide a stable public ABI. Future
out-of-process extensions may be evaluated if a real interoperability need
emerges.

## Revisit conditions

Reconsider this decision if measured deployments require coordinated
high-availability refresh, durable warm starts, independently scaled providers,
or a stable external collector protocol. Any replacement must preserve the
rule that Prometheus scrapes never initiate cloud billing API calls.
