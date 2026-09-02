# Secure Software Development Lifecycle

**Product:** Atmos CLI (`github.com/cloudposse/atmos`)
**Owner:** Cloud Posse, LLC
**Version:** 1.0
**Review cadence:** Annually, and after any material change to the pipeline

---

## 1. Purpose and Scope

This document describes how Cloud Posse develops, reviews, builds, and releases the Atmos CLI. It covers the public repository, its CI/CD pipeline, and its release artifacts.

It does not cover Atmos Pro, a separate hosted SaaS offering with its own controls and its own SOC 2 programme.

Cloud Posse develops Atmos in the open. Every control described here is visible in the public repository and can be verified independently. Assertions in this document are inspectable, not self-reported.

## 2. Roles

| Role | Responsibility |
|---|---|
| Maintainers | Review and approve changes; own the code areas assigned in `CODEOWNERS` |
| Release manager | Publishes the draft release that triggers the build and signing pipeline |
| Organization administrators | Manage repository rulesets and organization security settings. Two people hold this role. |
| External contributors | Propose changes from forks. They hold no write access. |

Code ownership is declared in `.github/CODEOWNERS`. Default ownership is `@cloudposse/engineering`. The `CODEOWNERS` file itself and the merge automation configuration escalate to `@cloudposse/admins`.

## 3. Development Flow

Development is trunk-based on `main`. Atmos follows GitHub Flow.

1. **Branch.** A maintainer creates a branch. An external contributor creates a fork.
2. **Implement.** The contributor writes the code and its tests.
3. **Open a pull request.** Direct commits to `main` are not possible. No identity holds bypass permission on the pull-request requirement.
4. **Automated checks run.** See Section 4.
5. **AI review runs.** CodeRabbit reviews every pull request automatically.
6. **Human review.** At least one approving review is required, and review from a code owner is required. Approval of the most recent reviewable push is required, which prevents an author from approving their own change. Stale approvals are dismissed when new commits are pushed. Every review conversation must be resolved before merge.
7. **Label.** Each pull request must carry a release-impact label: `patch`, `minor`, `major`, or `no-release`. A status check enforces this before merge. The label is not administrative: it drives the next version number, as described in Section 6.
8. **Merge queue.** An approved pull request enters a merge queue. The queue re-runs the required checks against the merge result. A change that fails at this stage does not reach `main`.
9. **Merge.** On success the change merges to `main`, and the draft release and its notes update automatically.

## 4. Automated Gates

### 4.1 Blocking

A pull request cannot merge while any of these fail.

- Build, across Linux, macOS, and Windows
- Lint
- Unit and acceptance tests
- Demo scenario matrix
- Release-label (semver) validation
- Tracked-symlink verification
- **CodeQL** code scanning, at a threshold of security alerts of High or higher and alerts of severity Error
- **Dependency review**, at moderate severity
- **License check**
- **TruffleHog** secret scanning across full history, verified results only
- **Action SHA-pin drift** check
- **Third-party attribution drift** check
- Project-level test-coverage status, which fails on a coverage drop of more than one percent

### 4.2 Advisory

These report findings to GitHub code scanning. They inform review. They do not block a merge today.

- Semgrep
- gosec
- govulncheck, which performs call-graph reachability analysis and reports whether Atmos calls an affected symbol
- Trivy container image scan, for vulnerabilities and secrets
- golangci-lint, which runs with a zero exit code
- Patch test coverage, target 85 percent on new and changed lines, currently informational

Cloud Posse states this division deliberately. Describing an advisory scanner as a gate would be inaccurate and is verifiable in the repository configuration.

## 5. Branch and Repository Protection

Six repository rulesets govern `main`. Each has enforcement status **Active** and an **empty bypass list**, so each binds every identity, administrators included.

1. Required pull-request reviews
2. Required status checks
3. Signed commits
4. Merge queue
5. Force-push and branch-deletion protection
6. CodeQL code-scanning results

No actor holds bypass permission on any ruleset. An administrator retains the ability to *modify* a ruleset, which is different from bypassing one: relaxing a protection requires editing the ruleset, and that edit is recorded in the organization audit log.

Five organization-level rulesets govern the tag namespace. A release tag can only be created by the release automation, cannot be moved or deleted once cut, and no arbitrary tag can be introduced alongside it.

Organization-wide settings: multi-factor authentication is required, personnel authenticate through Google Workspace single sign-on, commit signing is enforced, and secret scanning with push protection is enabled. Bypass of push protection is held by the organization-admin role alone.

## 6. Release Process

1. **release-drafter** maintains a **draft release** for the next version. On each merge to `main` it resolves the next semantic version from the pull-request labels described in Section 3, and it generates the changelog from the merged pull requests. Version resolution is therefore mechanical, and it derives from a label that a required status check enforced before merge.
2. **GoReleaser** builds the platform binaries from `main` and attaches them to that draft release.
3. The release manager publishes the draft. Publication is the human decision point.
4. Publication triggers the build pipeline. The pipeline runs in a protected `release` deployment environment whose deployment-branch policy admits only workflows running on `main`. A release cannot be produced from an untrusted branch.
5. The pipeline rebuilds the binaries from source at the published tag, rather than re-signing artifacts it downloaded. This closes the time-of-check to time-of-use gap present in download-then-sign designs.
6. The pipeline produces:
   - Platform binaries and native `.deb`, `.rpm`, and `.apk` packages
   - A SHA256 checksums manifest covering the release artifacts
   - A keyless Sigstore signature over the checksums manifest, recorded in the Rekor transparency log
   - SPDX SBOMs, per archive and for the source tree
   - Build-provenance attestations for every artifact
   - A container image, signed by digest with cosign and scanned with Trivy
7. Release binaries link Go's native FIPS 140-3 cryptographic module and default to FIPS-enforcing mode at runtime. This is not a CMVP certification for Atmos, and it does not cover the age and NaCl cryptography used by declarative secrets management.

Feature previews are prereleases cut from a pull-request branch through a separate `feature-releases` environment. They are temporary, are marked as prereleases, and are not production releases.

## 7. Third-Party Components

- Go modules resolve through the public Go module proxy. The proxy is a cache, not a trust anchor.
- Every module is pinned by cryptographic digest in the committed `go.sum` and verified against the `sum.golang.org` checksum transparency log on each fetch. A modified module fails the build.
- No checksum-bypass environment variable is configured anywhere in the repository, its workflows, its scripts, or its Dockerfile.
- Every GitHub Action is pinned to a commit SHA. A drift check enforces this.
- Dependency updates are proposed automatically and reviewed through the same pull-request process. Dependabot security updates are enabled.
- Dependency review blocks a pull request that introduces a dependency with a known vulnerability at moderate severity or above.

## 8. Vulnerability Management and Disclosure

- Reports are received at `security@cloudposse.com`. Cloud Posse acknowledges a report within 48 hours, provides an estimated remediation timeframe, and notifies the reporter on resolution.
- The published policy is at `github.com/cloudposse/atmos/security/policy`.
- Remediation targets: Critical within 7 days, or 72 hours where the vulnerability is reachable in Atmos and an upstream fix is available; High within 30 days; Medium within 90 days; Low in the next regular release. Measured from confirmation.
- Confirmed vulnerabilities are disclosed through a GitHub Security Advisory and in the release notes.
- Cloud Posse supports coordinated disclosure. Cloud Posse does not operate a bug bounty.
- A security fix uses the same pipeline as any other change. Atmos releases frequently, so no exceptional path is required, and no emergency process bypasses review, testing, signing, or attestation.

## 9. Secrets

Three independent layers protect against credential exposure.

1. GitHub secret scanning with push protection, organization-wide, which blocks known credential formats at push time. Detection includes generic-pattern and AI-assisted detection beyond known provider formats.
2. TruffleHog across full repository history in CI, verified results only, blocking.
3. Atmos itself masks secrets in output and handles configured secrets through dedicated backends rather than environment interpolation.

CI jobs run on ephemeral runners. Runners provisioned through RunsOn execute in a dedicated AWS automation account.

## 10. Records

- Every change to Atmos is a public commit with a verified signature, attached to a reviewed pull request.
- Code-scanning results, dependency alerts, and their dispositions are recorded in the repository's Security tab. A dismissal carries a written justification.
- Organization audit logs record permission changes and ruleset modifications.
- Release artifacts and their attestations are retained on the GitHub Release and in the Rekor transparency log.

## 11. Mapping to NIST SP 800-218 (SSDF)

| SSDF practice | Where satisfied |
|---|---|
| PO.3 Supporting toolchains | Sections 4, 6 |
| PO.5 Secure build environments | Sections 5, 6, 9 |
| PS.1 Protect code from unauthorized change | Sections 3, 5 |
| PS.2 Provide a mechanism to verify integrity | Section 6 |
| PS.3 Archive and protect each release | Sections 6, 10 |
| PW.4 Reuse well-secured software | Section 7 |
| PW.6 Configure the build process for security | Section 6 |
| PW.7 Review human-readable code | Section 3 |
| PW.8 Test executable code | Sections 3, 4 |
| PW.9 Configure secure default settings | Section 6 |
| RV.1 Identify and confirm vulnerabilities | Sections 4, 8 |
| RV.2 Assess, prioritize, and remediate | Section 8 |
| RV.3 Analyze vulnerabilities to identify root causes | Section 8 |

## 12. Known Limitations

Cloud Posse states these rather than leaving them to discovery.

- Several scanners operate in advisory mode. Section 4.2 lists them.
- Patch-level coverage is a target, not a gate.
- Formal incident-response, access-control, and personnel-security policies are not yet written. They are being produced under the SOC 2 programme Cloud Posse is pursuing for Atmos Pro.
- Multi-factor authentication is enforced but is not restricted to phishing-resistant factors.
- The build attains SLSA Build Level 2.
