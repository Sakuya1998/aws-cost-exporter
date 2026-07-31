[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Architecture) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Architecture-zh-CN)

# Architecture

AWS Cost Exporter is a modular monolith with inward-pointing dependencies. Domain packages do not import the AWS SDK, Prometheus, HTTP, Cobra, or Viper.

## Modules

- `internal/domain`: target identity and immutable cost, budget, commitment, anomaly, organization, tag, and aggregate snapshot values.
- `internal/ports`: narrow interfaces consumed by collectors and the application.
- `internal/aws`: SDK clients, credential composition, AssumeRole, limiters, pagination, mapping, and safe error classification.
- `internal/collector`: maps reader ports into typed `PartialSnapshot` results.
- `internal/scheduler`: intervals, target-scoped single-flight, serial target execution, global concurrency, and bounded failure backoff.
- `internal/cache/memory`: copy-on-write partial maps and atomic aggregate publication.
- `internal/metrics`: fixed Prometheus descriptors and snapshot traversal.
- `internal/httpserver`: metrics, probes, version, and optional diagnostics.
- `internal/app`: composition root.

## Identity

```go
type TargetID string

type CollectorID struct {
    Target TargetID
    Name   string
}
```

Every domain value, cache partial, status, scheduler job, log event, and target-level metric carries target identity. Costs also carry bounded provider and basis values, preventing Cost Explorer and CUR or alternative accounting views from being silently merged.

## Runtime flow

```text
base AWS config
  -> target credential / AssumeRole cache
  -> target AWS clients
  -> typed collectors
  -> shared scheduler
  -> copy-on-write cache rebuild
  -> atomic immutable snapshot
  -> Prometheus collectors
  -> HTTP server
```

Prometheus scrape reads one atomic pointer without application locks and iterates values without copying complete slices. A target/collector failure updates only its status and retains the previous successful partial.

## AWS attempt policy

```text
global limiter -> target limiter -> SDK attempt token -> HTTP request
```

The wrapper is installed in the SDK attempt-token path, so initial requests and retries are both limited without replacing SDK retry/backoff/token-bucket behavior. Cancellation reaches waits, pagination, Athena polling, backoff, and workers.

## Decisions

- Querying AWS on scrape was rejected because it couples Prometheus to paid, paginated APIs.
- Persistent local cache was deferred because Prometheus owns retention and persistence adds migration/encryption/HA concerns.
- Dynamic Go plugins were rejected; collectors use compile-time registration and narrow ports.
- v0.3.0 remains single-replica. See [ADR 0002](https://github.com/Sakuya1998/aws-cost-exporter/blob/master/docs/adr/0002-ha-refresh-coordination.md).

The authoritative decision records are in [`docs/adr`](https://github.com/Sakuya1998/aws-cost-exporter/tree/master/docs/adr).

## v1 operational envelope

v1 supports 20 targets and 20,000+ business series in one process on the 2 vCPU
and 512 MiB reference environment. Snapshots remain memory-only snapshots:
restart loses cached values and Prometheus owns durable retention. There is no
leader election, fencing, shared persistence, or coordinated multi-replica
refresh. The Helm replacement strategy prevents old and new pods from making
paid requests at the same time.
