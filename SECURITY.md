# Security Policy

## Supported versions

The latest `v0.x` release line receives security fixes. Pre-1.0, only the most
recent minor is supported.

## Reporting a vulnerability

Please report suspected vulnerabilities privately rather than opening a public
issue:

- Use GitHub's **"Report a vulnerability"** (Security → Advisories) on this
  repository, or
- email **krebbekx@gmail.com**.

You'll get an acknowledgement within a few days. Please include a description,
affected version, and a minimal reproduction if possible.

## Supply-chain hardening

- GitHub Actions are pinned to commit SHAs and updated via Dependabot.
- Workflows run with least-privilege `contents: read` permissions and do not
  persist credentials.
- Module dependencies are pinned via `go.sum` and tracked by Dependabot.
