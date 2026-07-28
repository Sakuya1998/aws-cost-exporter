[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/CUR-and-Athena) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/CUR-and-Athena-zh-CN)

# CUR 2.0 and Athena

CUR uses `provider="cur_athena"`. The exporter does not create Data Exports, S3 buckets, Glue databases/tables, Athena workgroups, or partitions. Prepare them before enabling the target.

## Data flow

```text
AWS Billing Data Exports
  -> CUR 2.0 files in an S3 input prefix
  -> Glue Data Catalog table
  -> Athena workgroup
  -> exporter fixed aggregate query
  -> immutable Prometheus snapshot
```

The CUR input prefix and Athena result output prefix are different responsibilities. The exporter reads CUR objects and Athena writes query results under `output_location`.

```yaml
targets:
  - name: payer-prod
    account_id: "444455556666"
    cur:
      enabled: true
      region: us-east-1
      database: billing
      table: cur2
      workgroup: aws-cost-exporter
      output_location: s3://example-athena-results/aws-cost-exporter/
      query_timeout: 10m
      poll_interval: 2s
      tag_columns:
        - key: Environment
          column: resource_tags_user_environment
```

The Athena workgroup ARN region must match `targets[].cur.region`, and the Glue catalog must exist in that region. This region is independent from the Cost Explorer `aws.region`.

## Query lifecycle

The exporter generates fixed, read-only aggregate SQL and does not accept arbitrary SQL or templates.

```text
StartQueryExecution
  -> GetQueryExecution polling
  -> SUCCEEDED
  -> GetQueryResults pagination
  -> schema, row, currency and uniqueness validation
  -> atomic publish
```

Only `SUCCEEDED` results publish. `FAILED`, `CANCELLED`, timeout, malformed metadata, duplicate pagination tokens, row/page/series overflow, or currency-limit failure retain the last snapshot. Any abnormal exit before a terminal Athena state performs best-effort `StopQueryExecution`.

One deadline covers submission, polling, and pagination. Polling and SDK retries use the global and target limiters.

## Cost semantics

The fixed queries support daily and month-to-date totals, allowlisted tags, and unblended/amortized/net bases. CUR amortized logic handles Savings Plans and RI effective cost and unused commitment rows while excluding negation/upfront rows that would double count. Net uses net unblended cost with documented fallback when AWS leaves it empty.

Returned columns, types, dates, currencies, rows, pages, and duplicate label sets are validated before publication. `max_currencies` bounds the union across total and tag results.

## Athena cost control

Athena charges by bytes scanned. Partition the CUR table by billing period, use a dedicated workgroup with scan limits, keep the default 24-hour refresh unless necessary, and monitor workgroup metrics. Prometheus scrapes never submit Athena queries.

Use the scoped [`cur-athena-readonly.json`](https://github.com/Sakuya1998/aws-cost-exporter/blob/master/examples/iam/cur-athena-readonly.json) policy after replacing every account, region, database, table, bucket, and prefix placeholder.
