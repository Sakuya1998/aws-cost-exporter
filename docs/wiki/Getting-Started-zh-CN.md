[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Getting-Started) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Getting-Started-zh-CN)

# 快速开始

下面使用 AWS 默认凭证链启动一个 required Cost Explorer target。

## 前置条件

- Go 1.24、已发布二进制或容器镜像。
- 默认凭证链提供账户 `444455556666` 的凭证。
- 具有 `ce:GetCostAndUsage` 和 `ce:GetCostForecast` 权限。
- 能够访问 AWS API。

复制 `configs/aws-cost-exporter.example.yaml`，替换占位 Account ID。第一次运行时保持可选 Collector 关闭。

```yaml
aws:
  region: us-east-1
  credentials:
    sources:
      runtime:
        type: default_chain

targets:
  - name: payer-prod
    account_id: "444455556666"
    required: true
    credentials:
      source: runtime
    cost_explorer:
      enabled: true
      filters:
        linked_account_ids: []
        services: []
        regions: []
```

启动前先校验：

```bash
./aws-cost-exporter --config config.yaml --check-config
./aws-cost-exporter --config config.yaml
```

检查 HTTP 端点：

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/ready
curl http://localhost:8080/metrics
curl http://localhost:8080/version
```

在 required target 的全部 Cost Explorer collector 第一次成功前，`/ready` 可以返回 503。先按 `target`、`collector` 查看日志以及 `aws_cost_exporter_collector_up`，不要直接提高重试或限流参数。

后续逐个加入 Profile、AssumeRole target、过滤器和可选 Collector。每次改动后都重新运行 `--check-config`。
