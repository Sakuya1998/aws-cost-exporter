[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/CUR-and-Athena) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/CUR-and-Athena-zh-CN)

# CUR 2.0 与 Athena

CUR 使用 `provider="cur_athena"`。Exporter does not create Data Exports、S3 Bucket、Glue Database/Table、Athena Workgroup 或 Partition，启用前必须预先创建。

## 数据链路

```text
AWS Billing Data Exports
  -> S3 输入 prefix 中的 CUR 2.0 文件
  -> Glue Data Catalog Table
  -> Athena Workgroup
  -> Exporter 固定汇总查询
  -> immutable Prometheus snapshot
```

CUR 输入 prefix 与 Athena 查询结果 prefix 是不同用途。Exporter 读取 CUR object，Athena 将结果写入 `output_location`。

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

Athena Workgroup 与 Glue Catalog 必须位于 `targets[].cur.region`，它与 Cost Explorer 的 `aws.region` 相互独立。

## 查询生命周期

Exporter 只生成固定只读汇总 SQL，不接受 arbitrary SQL 或用户模板。

```text
StartQueryExecution
  -> GetQueryExecution 轮询
  -> SUCCEEDED
  -> GetQueryResults 分页
  -> schema、行数、currency、唯一性校验
  -> 原子发布
```

只有 `SUCCEEDED` 才发布。`FAILED`、`CANCELLED`、超时、元数据异常、重复 pagination token、行/页/series 超限或 currency 超限都会保留旧 snapshot。Athena 尚未进入终态时异常退出，会 best-effort 调用 `StopQueryExecution`。

一个 deadline 覆盖提交、轮询和分页；轮询及 SDK retry 都使用全局与 target limiter。

## 成本语义

固定查询支持 daily、month-to-date、allowlist Tag 以及 unblended/amortized/net basis。CUR amortized 逻辑处理 Savings Plans、RI effective cost 和未使用承诺行，并排除会重复计算的 negation/upfront 行。Net 使用 net unblended cost，在 AWS 留空时执行文档化 fallback。

返回列、类型、日期、currency、行数、页数和重复 label set 在发布前严格校验。`max_currencies` 限制 total 与 Tag 结果的 currency 并集。

## Athena 成本控制

Athena 按扫描字节计费。建议按账期分区 CUR、使用设置扫描上限的专用 Workgroup、保持默认 24 小时刷新并监控 Workgroup。Prometheus scrape 永远不会提交 Athena 查询。

使用限定范围的 [`cur-athena-readonly.json`](https://github.com/Sakuya1998/aws-cost-exporter/blob/master/examples/iam/cur-athena-readonly.json)，并替换所有 Account、Region、Database、Table、Bucket 和 Prefix 占位值。
