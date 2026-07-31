# Security Policy

## Supported versions

v1.x is the long-lived stable line after v1.0.0 is published. The latest two
v1 minor releases receive security fixes; an older minor remains supported for
six months after its successor is released. Until v1.0.0 is published, v0.3.x
remains the supported release line. The default branch is development code and
is not a supported release.

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

## Coordinated disclosure

The project follows coordinated disclosure: maintainers validate the report,
prepare a fix and advisory, and agree on a publication date with the reporter.

Maintainers will validate the report, determine affected versions, prepare a
fix and advisory, and coordinate a release date with the reporter. Please allow
reasonable time for users to upgrade before publishing technical details.
Critical issues may require an accelerated release.

Security releases include a GitHub advisory, fixed version, impact statement,
upgrade instructions, and mitigations when an immediate upgrade is impossible.
Credit is given unless the reporter requests anonymity.

## Release artifact revocation and replacement

If a published binary, image, chart, SBOM, provenance statement, signature, or
workflow identity is compromised, maintainers will mark the affected release
and advisory, stop recommending the affected artifacts, and revoke access or
distribution where the hosting platform supports it. Immutable tags and audit
records are not silently rewritten.

Maintainers will build replacement release artifacts from a reviewed clean
commit under a new patch version, repeat supply-chain verification, publish new
digests and cosign commands, and identify every superseded artifact. Consumers
must upgrade or pin the documented unaffected digest. The security advisory
records the reason, affected interval, replacement version, and any credential
or signing-identity rotation required.

## Security scope

Relevant reports include credential exposure, unauthorized AWS API access, IAM
privilege escalation, unsafe debug endpoints, sensitive cost-data disclosure,
container or Helm privilege issues, dependency vulnerabilities with a viable
attack path, and denial of service through unbounded metric cardinality.

General support questions, expected AWS Cost Explorer data latency, inaccurate
cost interpretation without a security impact, and vulnerabilities that require
already-authorized administrative access are normally handled as regular issues.
