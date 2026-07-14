# Contributing to aws-cost-exporter

Thank you for helping improve aws-cost-exporter. Contributions of code,
documentation, tests, issue reports, and design feedback are welcome.

Participation in this project requires following [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).
Security vulnerabilities must be reported through the private process in
[SECURITY.md](SECURITY.md), not through a public issue.

## Before opening a change

Search existing issues and pull requests first. Open an issue before work that
changes public metrics, labels, configuration, HTTP APIs, IAM requirements, or
architecture. Small fixes and documentation corrections can go directly to a
pull request.

Keep each change focused. Do not combine unrelated refactoring with a feature or
fix. Public metric and configuration compatibility is part of the API and must
include migration notes when changed.

## Development environment

Required tools:

- Go 1.24 or newer.
- GNU Make.
- golangci-lint.
- Git with commit signing identity configured as required by your organization.

Run the local quality checks before submitting:

```sh
make build
make test
make lint
```

Use standard Go formatting and conventions. New behavior requires tests written
before the implementation. Keep interfaces narrow and define them in the
package that consumes them. Do not add static AWS credentials, account data, or
unredacted Cost Explorer responses to source code, fixtures, logs, or issues.

## Pull requests

1. Fork [github.com/sakuya1998/aws-cost-exporter](https://github.com/sakuya1998/aws-cost-exporter) and create a branch from the current default branch.
2. Add tests and documentation for externally observable behavior.
3. Run all applicable local checks.
4. Use a clear pull request title and explain motivation, behavior, risk, and
   verification.
5. Keep generated files in a separate commit when practical.
6. Address review comments with additional commits; maintainers may squash when
   merging.

Commit messages should follow Conventional Commits, for example
`feat(collector): add service cost collection` or
`fix(cache): preserve the last successful snapshot`.

## Developer Certificate of Origin

Every commit must certify the
[Developer Certificate of Origin 1.1](https://developercertificate.org/) with a
`Signed-off-by` trailer:

```sh
git commit --signoff -m "feat(scope): describe the change"
```

The sign-off confirms that you have the right to submit the contribution under
the project's Apache-2.0 license. It is not a copyright assignment.

## Review and acceptance

Maintainers evaluate correctness, test quality, compatibility, operational
risk, security, and fit with the roadmap. Passing automation does not guarantee
acceptance. Reviewers may request that a large change be split or documented in
an architecture decision record.

Contributions are accepted when required checks pass, review discussions are
resolved, documentation is current, and a maintainer approves the change.
