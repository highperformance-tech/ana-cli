# Security Policy

## Reporting a vulnerability

Please do **not** open a public issue for security-sensitive reports.

Email `security@highperformance.tech` with:

- A description of the issue and the impact you can reproduce.
- Steps to reproduce (command, environment, observed vs. expected).
- Whether the vulnerability involves credentials, a release artifact,
  the install script, or the CLI itself.

You will receive an acknowledgement within three business days. We
coordinate disclosure privately and credit reporters in the release
notes unless anonymity is requested.

## Scope

In scope:

- The `ana` binary and any code under `cmd/` or `internal/`.
- The release pipeline (GoReleaser, release-please) and the
  published archives / checksums.
- `install.sh` and anything it fetches or verifies.

Out of scope:

- Vulnerabilities in the TextQL server surface that `ana` talks to.
  Report those directly to TextQL.
- Third-party dependencies with upstream advisories already filed.

## Supported versions

Only the latest minor release receives security fixes. Older tags are
left in place for reproducibility but are not patched.
