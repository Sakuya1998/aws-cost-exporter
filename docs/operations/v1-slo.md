# v1 Capacity and Stability SLO

AWS Cost Exporter v1 supports one process with 20 targets and at least 20,000
business series on a reference allocation of exactly 2 vCPU and no more than
512 MiB memory. The `/metrics` objective is p99 latency below 5 seconds and at
least 99.9% successful scrapes during the 24-hour release-candidate soak.

The workload is entirely in memory: 20 deterministic targets expose at least
1,000 cost and forecast series each. It does not configure AWS credentials and
does not call AWS or Athena. AWS API latency, Athena query latency, Prometheus
network failures, and scrape scheduling outside the exporter are excluded from
this implementation SLO.

The release workflow scrapes every 15 seconds for 24 hours on the dedicated
`aws-cost-exporter-stability` runner. It records the source SHA, Go version,
kernel, logical CPU count, cgroup memory limit, business-series count, scrape
count, errors, p99, heap/RSS growth, goroutine growth, and open-connection
growth. The report contains no target account IDs, credentials, costs, SQL, or
AWS responses.

The gate warms the registry before its baseline. It permits at most 32 MiB Go
heap growth, 64 MiB RSS growth, two goroutines, and two connections. To detect
sustained growth rather than startup noise, least-squares slopes are calculated
from the second half of the run and must remain below 4 MiB/hour heap,
8 MiB/hour RSS, one goroutine/hour, and one connection/hour. Short local runs
record the same values but do not enforce slope thresholds.

Run a two-minute local validation with an explicit output path:

```powershell
$env:AWS_COST_EXPORTER_STABILITY = "1"
$env:AWS_COST_EXPORTER_STABILITY_DURATION = "2m"
$env:AWS_COST_EXPORTER_STABILITY_INTERVAL = "1s"
$env:AWS_COST_EXPORTER_STABILITY_OUTPUT = "$env:TEMP\aws-cost-exporter-stability.json"
go test ./test/perf -run '^TestV1StabilitySoak$' -count=1 -v
```

Only the manual 24-hour workflow is release evidence. A local run is a harness
check and must not be cited as v1.0.0 acceptance.
