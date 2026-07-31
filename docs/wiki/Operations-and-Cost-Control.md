[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Operations-and-Cost-Control) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Operations-and-Cost-Control-zh-CN)

# Operations and Cost Control

## Probes and snapshots

- `/healthz` reports only process liveness.
- `/ready` requires every enabled Cost Explorer collector on every required target to have a fresh successful snapshot.
- Optional targets and Organizations, Budgets, commitments, anomalies, tags, and CUR do not gate readiness.
- `/metrics` serves the current immutable snapshot and never calls AWS or Athena.

A failed refresh records status and retains the last successful snapshot. Use `collector_up`, last-success timestamps, and cache age together; an old value can still be exposed after a failure.

## Scheduling and failure isolation

Collectors have independent intervals, single-flight state, bounded failure backoff, and `CollectorID`. One target runs its collectors serially so it cannot occupy every global slot. The process-wide semaphore enforces `collection.max_concurrency`.

Context cancellation stops limiter waits, AWS requests, Athena polling, SDK retries, backoff timers, and workers. Investigate `aws_cost_exporter_scheduler_shutdown_timeouts_total` when shutdown exceeds `server.shutdown_timeout`.

## Replica policy

Keep Helm `replicaCount: 1`. v1.0.0 has no leader election, shared cache, or refresh ownership. Multiple replicas duplicate paid requests and appear as duplicate Prometheus scrape targets.

## Cost Explorer cost

AWS currently charges approximately USD 0.01 per billable Cost Explorer request. Pagination and SDK retry attempts can increase cost. Monitor logical requests, retries, and successfully read pages, but treat AWS billing as authoritative.

Before increasing RPS or page limits:

1. Add target filters.
2. Disable unused collectors or bases.
3. Increase refresh intervals.
4. Reduce Tag keys and value budgets.
5. Inspect throttling and pagination alerts.

## Athena cost

Athena charges by scanned bytes. Use CUR partitions, a dedicated workgroup, scan limits, a narrow result prefix, and the default 24-hour interval. Totals and Tag collection are separate queries, but each fixed query references the CUR table once. Prometheus scrape frequency does not affect Athena query frequency.

## Freshness

Cost Explorer and CUR update on different schedules. `cache.freshness_ttl` controls expected freshness and `cache.stale_after` controls readiness staleness. Never hide provider differences by summing them.

## v1 capacity and SLO

The supported reference workload is 20 targets and at least 20,000 business
series on 2 vCPU and 512 MiB. The `/metrics` objective is p99 under 5 seconds
with at least 99.9% successful scrapes during the manual 24-hour stability run.
AWS and Athena latency are upstream exclusions because scrapes use only cached
data. The release report also evaluates sustained heap, RSS, goroutine, and
connection growth.

## Upgrade, shutdown, and backup

The chart fixes `replicaCount: 1`, `maxSurge: 0`, and `maxUnavailable: 1`.
Upgrades are cost-first: they avoid overlapping paid requests and intentionally
cause a temporary metrics outage until the new pod refreshes and becomes ready.
Shutdown marks `/ready` unavailable with `shutting_down` before canceling
collectors and draining HTTP.

Snapshots are memory-only snapshots and are not a backup target. Back up the
reviewed configuration and Secret references through the deployment platform;
protect Prometheus storage for historical metrics. Rollback restores the prior
binary and configuration but cannot restore an expired in-process snapshot.
