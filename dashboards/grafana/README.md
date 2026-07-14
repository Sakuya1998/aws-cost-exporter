# Grafana dashboard validation

Import `aws-cost-exporter.json` and bind the `DS_PROMETHEUS` input to the
Prometheus data source that scrapes the exporter.

The dashboard targets Grafana schema version 39 and uses only built-in Stat,
Time series, Bar gauge, Table, and Text panels. It has no plugin dependency.

Validation covers:

- valid JSON and stable dashboard metadata;
- all required variables with `All` support;
- all required cost and exporter-health panels;
- PromQL metric names against the exporter contract;
- `job`, `instance`, and `currency` filters on every business query;
- currency-preserving aggregation;
- counter-safe dimension-overflow queries.

For a release screenshot, use the UTC timezone and capture the complete
dashboard with the currency selector visible. Use synthetic or sanitized cost
data only; screenshots must not expose account IDs, service filters, or costs
from a private AWS account.

Billing data can arrive late and be backfilled. The daily time series shows
Prometheus scrape history for the current UTC billing-day gauge, not historical
Cost Explorer rows. Forecast metrics cover today through month end; estimated
month-end panels subtract today's amount from MTD before adding that forecast,
so the overlapping UTC billing day is not counted twice.
