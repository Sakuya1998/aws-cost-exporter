# IAM examples

These policies are capability-specific templates. Replace placeholder account
IDs, role names, workgroups, databases, tables, buckets, prefixes, budget names,
principals, and ExternalId values before use. Attach only the policies required
by collectors enabled for a target.

- `mvp-readonly.json` covers Cost Explorer cost and forecast reads.
- `organizations-readonly.json` covers account metadata reads.
- `budgets-readonly.json` scopes `budgets:ViewBudget` to one named budget.
- `commitments-anomalies-readonly.json` covers bounded Cost Explorer summaries.
- `cur-athena-readonly.json` scopes Athena, Glue, input S3, and result S3 access.
- `assume-role-permissions.json` permits `sts:AssumeRole` for one exact role.
- `assume-role-trust.json` trusts one exact runtime principal and ExternalId.

Cost Explorer and Organizations read actions in these examples use
`Resource: *`. AWS does not support resource-level permissions for the required
operations. This wildcard does not authorize write actions.
Do not replace any exact `sts:AssumeRole`, Athena, Glue, S3, or Budgets resource
with `*`.

ExternalId is not a secret substitute, but its value must still be injected by
an environment variable or Kubernetes Secret and must not be committed. Review
CloudTrail, Access Analyzer, the target account ID, and enabled collectors when
adapting a policy.
