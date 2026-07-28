[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Development-and-Testing) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Development-and-Testing-zh-CN)

# Development and Testing

## Environment

- Go 1.24 or newer; CI tests Go 1.24.x and stable.
- GNU Make, Git, Docker Buildx, Helm, and golangci-lint for the corresponding gates.
- Optional local `promtool` and `kubeconform`; CI installs pinned versions.

```bash
git clone https://github.com/Sakuya1998/aws-cost-exporter.git
cd aws-cost-exporter
go mod download
make build
```

## Quality gates

```bash
go test ./...
go test -race ./...
go vet ./...
golangci-lint run
```

CI additionally runs formatting/import checks, govulncheck, gosec, coverage at or above 79%, chart/dashboard/rule/docs tests, container smoke, and multi-architecture builds.

## Testing strategy

- Domain tests validate sorting, uniqueness, amount conservation, provider/basis identity, and immutable traversal.
- AWS adapter tests use fake endpoints for pagination, retry, cancellation, malformed results, and sensitive-data redaction.
- Scheduler/cache tests cover single-flight, target isolation, old-data retention, shutdown, and goroutine recovery.
- Golden metrics lock names, types, and fixed label order.
- Integration/E2E tests verify multiple targets, readiness, binary shutdown, and that `/metrics` never calls AWS.
- Asset tests validate Helm, kubeconform, dashboard PromQL, rules, IAM, release configuration, and Wiki contracts.

Write a failing test before changing behavior. Keep interfaces narrow and defined in the consuming package. Do not introduce AWS SDK response types into domain packages or arbitrary labels into Prometheus descriptors.

## Pull requests

Follow [`CONTRIBUTING.md`](https://github.com/Sakuya1998/aws-cost-exporter/blob/master/CONTRIBUTING.md), keep changes focused, explain operational risk, update docs for public contracts, and sign commits with the Developer Certificate of Origin:

```bash
git commit --signoff -m "docs: describe the change"
```

Security reports use the private process in `SECURITY.md`, not public issues.
