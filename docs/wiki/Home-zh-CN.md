[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Home) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Home-zh-CN)

# AWS Cost Exporter Wiki

本 Wiki 只描述当前稳定版本 **v0.3.0**。

v1.0 稳定化正在进行，不增加 Collector。它将当前配置、指标与 HTTP
行为冻结为 v1 契约，并增加可机器校验的兼容性、生命周期、容量、安全和
发布门禁。在真实验收证据齐全之前，可安装的稳定版本仍是 v0.3.0。

AWS Cost Exporter 将低频更新的 AWS 成本数据转换为稳定、带 `target` 维度的 Prometheus 指标。它是成本可观测性 Exporter，不是财务对账系统，AWS 仍然是最终数据源。

## 支持能力

- 显式多账户 target，支持默认凭证链、Profile、环境变量静态凭证和 AssumeRole。
- Cost Explorer 总成本、Service、Region、Linked Account、预测和 allowlist Tag 成本。
- `unblended`、`amortized`、`net` 成本基准，并明确携带 `provider` 与 `cost_basis`。
- 通过 Athena 查询 CUR 2.0 汇总数据。
- Organizations 账户元数据、COST Budget、Savings Plans/RI 汇总和成本异常汇总。
- Prometheus Rules、Grafana Dashboard、Docker 镜像、Helm OCI Chart 和最小权限 IAM 示例。

## 核心运行契约

后台 Collector 按受控周期调用 AWS 和 Athena。成功的 partial snapshot 经过校验后原子发布到内存，`/metrics` 只读取 immutable snapshot，不会访问 AWS 或 Athena。

某个 target 刷新失败时会保留该 collector 上一次成功数据，其他 target 继续运行。`/healthz` 只表示进程存活；`/ready` 要求 required target 的 Cost Explorer 数据成功且未过期。

## 从这里开始

1. 阅读[快速开始](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Getting-Started-zh-CN)。
2. 选择[安装方式](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Installation-zh-CN)。
3. 了解[凭证与 target 身份验证](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Credentials-and-Targets-zh-CN)。
4. 使用 `--check-config` 校验生产配置。
5. 第一次刷新成功后导入 Dashboard 和告警规则。

除非明确接受重复 AWS/Athena 请求，否则保持单副本。v1 契约保持
`replicaCount: 1`、仅内存 Snapshot，并且不实现 Leader Election 或共享缓存。
