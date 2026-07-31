[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Installation) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Installation-zh-CN)

# 安装

v1 的 Helm 还固定 RollingUpdate 参数 `maxSurge: 0` 和 `maxUnavailable: 1`，
避免新旧 Pod 重叠产生付费 AWS 请求。替换 Pod 启动、刷新仅内存 Cache 并 Ready
之前会出现临时指标中断。执行 `helm upgrade` 前先用新 Binary 校验当前配置；如果
Readiness 无法恢复则执行 `helm rollback`。单副本不提供零停机升级模式。

## Release 产物

从 [GitHub Release v0.3.0](https://github.com/Sakuya1998/aws-cost-exporter/releases/tag/v0.3.0) 下载归档和 checksum。Release 包含 Linux、Windows、macOS 的 amd64/arm64 归档、SPDX JSON SBOM 和 `checksums.txt`。

## 从源码构建

```bash
git clone https://github.com/Sakuya1998/aws-cost-exporter.git
cd aws-cost-exporter
git checkout v0.3.0
make build
./aws-cost-exporter --version
```

## Docker

```bash
docker pull ghcr.io/sakuya1998/aws-cost-exporter:0.3.0
docker compose up --build
```

只读挂载 AWS shared configuration，或者注入凭证源所引用的环境变量。不要把真实凭证构建进镜像。

## Helm

```bash
helm install aws-cost-exporter \
  oci://ghcr.io/sakuya1998/charts/aws-cost-exporter \
  --version 0.3.0 \
  --set config.data.targets[0].account_id=444455556666
```

使用 `config.secretEnvRefs` 引用已有 Secret，使用 `awsSharedConfig.existingSecret` 挂载 AWS Profile 文件。Chart 默认并要求优先保持 `replicaCount: 1`。

## 验证 OCI 签名

镜像和 Helm Chart 由 tag 触发的 Release workflow 进行 keyless 签名。请使用 [v0.3.0 验证记录](https://github.com/Sakuya1998/aws-cost-exporter/blob/master/docs/releases/v0.3.0-verification.md)中的精确 identity 与 issuer。需要不可变部署时固定已验证 digest。
