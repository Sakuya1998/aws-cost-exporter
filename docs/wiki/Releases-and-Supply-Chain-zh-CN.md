[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Releases-and-Supply-Chain) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Releases-and-Supply-Chain-zh-CN)

# 发布与供应链

Wiki 只描述当前稳定版本。历史配置契约通过 Git Tag 和 GitHub Release 保留，不维护并行的旧版 Wiki 树。

## 发布流程

1. PR 通过 CI 和 Review 后合并到 `master`。
2. 从已验证的 `master` commit 创建 annotated SemVer tag，例如 `v0.3.0`。
3. Tag 触发 Release workflow，校验 SemVer 并安装固定版本工具。
4. Trivy 扫描每个平台镜像的 HIGH/CRITICAL 问题。
5. Buildx 发布带 Provenance 与 SBOM 的多架构镜像。
6. Cosign 使用 GitHub Actions keyless identity 对镜像 manifest digest 签名。
7. Helm 打包并推送 OCI Chart，再由 cosign 对 digest 签名。
8. GoReleaser 构建归档、SPDX JSON SBOM、checksum，并创建 Draft Release。
9. Maintainer 独立验证签名与资产后正式发布 Draft。

Release workflow 拥有 `contents: write`、`packages: write`、`id-token: write`；普通 PR CI 保持只读。

## v0.3.0

- [GitHub Release](https://github.com/Sakuya1998/aws-cost-exporter/releases/tag/v0.3.0)
- [验证记录](https://github.com/Sakuya1998/aws-cost-exporter/blob/master/docs/releases/v0.3.0-verification.md)
- 镜像：`ghcr.io/sakuya1998/aws-cost-exporter:0.3.0`
- Chart：`ghcr.io/sakuya1998/charts/aws-cost-exporter:0.3.0`

验证记录固定了 Merge Commit、CI Run、13 个 Release 资产、Certificate Policy 和已验证镜像/Chart Digest。

## 历史证据

- [v0.1.5 验证](https://github.com/Sakuya1998/aws-cost-exporter/blob/master/docs/releases/v0.1.5-verification.md)
- [v0.1 发布清单](https://github.com/Sakuya1998/aws-cost-exporter/blob/master/docs/releases/v0.1-checklist.md)
- [全部 GitHub Releases](https://github.com/Sakuya1998/aws-cost-exporter/releases)

历史记录是不可变审计证据，不是当前安装或配置指南。

## 使用者验证

下载归档时校验 checksum。OCI 产物使用 Release Record 中的精确 Release Workflow Identity 和 GitHub Actions OIDC issuer 执行 cosign verify。不能接受 Tag 可变性时固定已验证 digest。
