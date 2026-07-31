[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Configuration-Reference) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Configuration-Reference-zh-CN)

# Configuration Reference

This page documents the exact v1.0.0 schema. The loader uses exact unmarshal: unknown, misspelled, deprecated, and legacy keys are rejected.

This schema is the stable v1 contract. The project guarantees v1.x backward compatibility:
patch and minor releases do not remove
or reinterpret accepted keys, defaults, enum values, or validation behavior.
Additive optional configuration may appear in a minor release. A deprecation is
documented for the latest two minor releases and at least six months before
removal in a future major release. There is no silent migration parser.

## Top-level keys

```text
server
log
aws
targets
collection
cache
telemetry
```

There is no `config_version` or compatibility parser.

## Validation workflow

```bash
./aws-cost-exporter --config config.yaml --check-config
```

The same validation and environment resolution run during production startup. Environment overrides use double underscores, for example `AWS_COST_EXPORTER_SERVER__LISTEN_ADDRESS=:9090`.

## Server and logging

`server` configures the listen address, `/metrics` path, positive HTTP timeouts, shutdown timeout, `max_in_flight` from 1 to 1000, and optional debug endpoints. Metrics, health, readiness, version, root, and enabled debug routes must not conflict. `log` selects `json` or `text`, severity, and optional source locations; logs always use stdout/stderr.

## AWS

`aws` configures Cost Explorer region, named credential sources, request timeout, STS/Cost Explorer/Organizations/Budgets/Athena test endpoints, retry policy, and rate limits.

- Credential sources: 1..20, unique names.
- `retry.max_attempts`: 1..10.
- Global and target burst: 1..5.
- RPS: finite, positive, at most 10.
- Retry delays and request timeout: positive and bounded.

## Targets

- Target count: 1..20.
- Name: `^[a-z][a-z0-9_-]{0,31}$` and unique.
- Account ID: exactly 12 digits and unique.
- At least one required target must enable Cost Explorer.
- Role ARN must be an exact IAM role ARN in the target account.
- Budgets names, Organizations account IDs, tag keys, and CUR tag columns must be unique.

## Collection

`collection` defines startup refresh, finite `jitter_ratio` from 0 to 0.5, global concurrency from 1 to 100, bounded failure backoff, and per-domain refresh intervals.

- Cost bases: unique non-empty subset of `unblended`, `amortized`, `net`.
- Cost Explorer and Organizations `series_limit`: at most 2000.
- Budgets `series_limit`: at most 500.
- `max_pages`: normally 1..100; CUR permits up to 200.
- Tag keys: at most 20; each `max_values` is 1..500.
- CUR: at most 100,000 rows, 1..10 `max_currencies`, and 20,000 series.
- `overflow_label` must already be trimmed and must not collide with provider/dimension values.

`targets[].cur.query_timeout` must exceed `aws.request_timeout`, remain at most one hour, and be shorter than `collection.cur.refresh_interval`. `poll_interval` is 100ms..1m. `output_location` requires a valid lower-case S3 bucket and non-empty prefix.

## Cache and telemetry

`cache.freshness_ttl` and `cache.stale_after` are positive; stale age must exceed the intended refresh interval. Telemetry controls the standard Go and process collectors.

Start from [`configs/aws-cost-exporter.example.yaml`](https://github.com/Sakuya1998/aws-cost-exporter/blob/master/configs/aws-cost-exporter.example.yaml); do not copy secrets into YAML.
