# Security Policy

## Supported Versions

| Version | Supported |
| ------- | --------- |
| latest release | ✅ |
| older releases | ❌ |

We only patch security issues on the latest release of ics-mcp. Please update to
the most recent tagged release before reporting a vulnerability.

## Reporting a Vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Instead, please use GitHub's Private Vulnerability Reporting:

1. Go to <https://github.com/jeeftor/ics-mcp/security/advisories/new>.
2. Click **Report a vulnerability**.
3. Fill in the advisory form with a description, affected versions, and a
   reproduction or proof-of-concept if available.

Reports submitted through GitHub Security Advisories are kept private and are
only visible to repository maintainers until a fix is coordinated and published.

## Response Timeline

- **Acknowledgment:** within **72 hours** of the initial report.
- **Status updates:** we will provide updates at least every 7 days until the
  issue is resolved or a mitigation is published.
- **Disclosure:** coordinated with the reporter before any public disclosure. A
  GitHub Security Advisory (GHSA) is published alongside the patch release.

## Supply-Chain Security

Each release of ics-mcp is accompanied by the following supply-chain artifacts:

- **SBOM** — a Software Bill of Materials in SPDX-JSON format is generated and
  attached to every GitHub release. It is also available as a workflow artifact
  from the `sbom` workflow run.
- **Build provenance attestations** — the Docker images published to
  `ghcr.io/jeeftor/ics-mcp` carry GitHub artifact attestations (build
  provenance and SBOM) that can be verified with
  [`gh attestation verify`](https://cli.github.com/manual/gh_attestation_verify).

To verify a Docker image attestation:

```bash
gh attestation verify ghcr.io/jeeftor/ics-mcp:<version> \
  --owner jeeftor
```

Continuous vulnerability scanning is performed via the `security` workflow,
which runs Trivy (filesystem CVE scan), `govulncheck` (Go vulnerability scan),
and `pnpm audit` (npm dependency audit) on every push, pull request, and on a
weekly schedule.
