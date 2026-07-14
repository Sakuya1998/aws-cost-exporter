# Security Policy

## Supported versions

Before v1.0, only the latest released minor version receives security fixes.
After v1.0, the latest minor release of the current major version is supported.
The default branch is development code and is not a supported release.

## Reporting a vulnerability

Use GitHub's **Report a vulnerability** form in the repository Security tab to
create a private security advisory. Do not open a public issue, pull request,
discussion, or chat message containing vulnerability details.

Include:

- Affected version or commit.
- Reproduction steps or a minimal proof of concept.
- Expected and observed behavior.
- Impact, attack prerequisites, and suggested severity.
- Any known mitigation or proposed fix.
- Whether public disclosure has occurred or is planned.

Remove AWS credentials, account identifiers, billing values, and other customer
data. If such data is necessary to reproduce the issue, describe its shape
rather than sending real values.

Maintainers aim to acknowledge a report within three business days, complete
initial triage within seven business days, and provide an update at least every
fourteen days while remediation is active. These are response targets rather
than service-level guarantees.

## Disclosure process

Maintainers will validate the report, determine affected versions, prepare a
fix and advisory, and coordinate a release date with the reporter. Please allow
reasonable time for users to upgrade before publishing technical details.
Critical issues may require an accelerated release.

Security releases include a GitHub advisory, fixed version, impact statement,
upgrade instructions, and mitigations when an immediate upgrade is impossible.
Credit is given unless the reporter requests anonymity.

## Security scope

Relevant reports include credential exposure, unauthorized AWS API access, IAM
privilege escalation, unsafe debug endpoints, sensitive cost-data disclosure,
container or Helm privilege issues, dependency vulnerabilities with a viable
attack path, and denial of service through unbounded metric cardinality.

General support questions, expected AWS Cost Explorer data latency, inaccurate
cost interpretation without a security impact, and vulnerabilities that require
already-authorized administrative access are normally handled as regular issues.
