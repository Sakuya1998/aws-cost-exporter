[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Optional-Collectors) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Optional-Collectors-zh-CN)

# Optional Collectors

Organizations, Budgets, commitments, anomalies, tags, and CUR are disabled by default. They do not gate `/ready`; only enabled Cost Explorer collectors on required targets do.

## Organizations

Uses `ListAccounts` and `DescribeOrganization` to enrich linked-account metrics with non-sensitive name and status. It never creates targets and never exports account email.

- Non-empty `account_ids`: export only the configured allowlist.
- Empty `account_ids`: export only accounts observed by the target account-cost collector.
- Observed-account mode requires the account collector.

## Budgets

Requires a non-empty, unique exact-name allowlist. Only `budget_type=COST` is accepted because usage and utilization budgets do not use currency units. Missing actual or forecast values omit those series rather than emitting zero.

## Savings Plans and Reserved Instances

Commitment collection reads utilization and coverage summaries for `savings_plan` and `reservation`. It publishes account-level ratios, unused hours, covered spend, on-demand equivalent, and net savings. Plan IDs, reservation IDs, services, and regions are deliberately excluded from labels.

## Cost Anomaly Detection

Anomaly collection reads paginated anomalies and publishes bounded summary signals: active state, count, cumulative impact, and latest detection timestamp. It never exports anomaly IDs, root causes, service names, regions, or raw AWS text.

## Tag costs

Every public tag key is an explicit allowlist entry with a per-key `max_values`. Cost Explorer queries each key separately. CUR additionally requires a unique safe SQL identifier mapping in `cur.tag_columns`.

Values beyond the bound aggregate into `__other__`; totals remain conserved. Startup validation rejects a worst-case tag budget that cannot fit `collection.tags.series_limit`. Duplicate label sets or hard-limit violations reject the complete refresh and retain old data.

## Failure behavior

Each optional collector has its own `CollectorID`, interval, status, single-flight state, backoff, and last successful partial snapshot. An authorization or timeout failure in one optional domain does not block another target or provider.
