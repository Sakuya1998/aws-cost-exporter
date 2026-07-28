[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Development-and-Testing) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Development-and-Testing-zh-CN)

# 开发与测试

## 环境

- Go 1.24 或更新版本；CI 测试 Go 1.24.x 和 stable。
- 对应门禁需要 GNU Make、Git、Docker Buildx、Helm、golangci-lint。
- 本地可选安装 `promtool`、`kubeconform`；CI 使用固定版本。

```bash
git clone https://github.com/Sakuya1998/aws-cost-exporter.git
cd aws-cost-exporter
go mod download
make build
```

## 质量门禁

```bash
go test ./...
go test -race ./...
go vet ./...
golangci-lint run
```

CI 还执行 formatting/import、govulncheck、gosec、不低于 79% 覆盖率、Chart/Dashboard/Rule/Docs 测试、容器 Smoke 和多架构构建。

## 测试策略

- Domain：排序、唯一性、金额守恒、Provider/Basis 身份、immutable 遍历。
- AWS Adapter：Fake Endpoint 覆盖分页、retry、取消、异常结果和敏感信息脱敏。
- Scheduler/Cache：single-flight、target 隔离、旧数据保留、shutdown、goroutine 回收。
- Golden Metrics：锁定名称、类型和固定 Label 顺序。
- Integration/E2E：多 target、readiness、二进制关闭以及 `/metrics` 不调用 AWS。
- Asset：Helm、kubeconform、Dashboard PromQL、Rules、IAM、Release 和 Wiki Contract。

修改行为前先写失败测试。接口保持窄并定义在消费方 Package。禁止让 AWS SDK Response 进入 Domain，禁止向 Prometheus Descriptor 加入任意 Label。

## Pull Request

遵循 [`CONTRIBUTING.md`](https://github.com/Sakuya1998/aws-cost-exporter/blob/master/CONTRIBUTING.md)，保持改动聚焦，解释运维风险，更新公开契约文档，并使用 Developer Certificate of Origin sign-off：

```bash
git commit --signoff -m "docs: describe the change"
```

安全漏洞必须通过 `SECURITY.md` 的私有流程报告，不要提交公开 Issue。
