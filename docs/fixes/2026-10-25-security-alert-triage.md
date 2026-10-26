# Security alert triage and remediation

**Date:** 2026-10-25

## Summary

The original 95-alert snapshot combined 17 Dependabot alerts with 78 code-scanning
alerts. Sixty-two code-scanning alerts were one Semgrep rule misclassifying GitHub's
same-repository `$/` action references. These references resolve to the exact workflow
commit; they are not mutable external tags.

A fresh query during implementation returned **90 open alerts: 11 Dependabot and
79 code-scanning**. Six DOMPurify/launch-editor alerts had closed upstream, the two
follow-redirects alerts had been replaced by #336/#337, and CodeQL #8581 was new.
This branch still needed the DOMPurify and launch-editor patches.

Changes address 77 findings from the original snapshot (62 false positives, 14
website dependency alerts and one Go standard-library alert). Against the refreshed
snapshot, they address 71 and leave 19 for further triage. These are expected effects
of merging and rescanning, not a claim that GitHub has closed the alerts already.
No remote alert dismissals were performed.

## Synchronization with main (2026-10-26)

Before publishing the PR, main already included the DOMPurify, follow-redirects
and launch-editor patches listed below. Merging main preserved those versions;
the final PR adds the body-parser, fast-uri and serialize-javascript patches
relative to main. The October 25 alert inventory and validation results remain a
historical snapshot rather than a live count or a claim that all six upgrades are
new in the final PR. The merged tree passes a frozen pnpm install and affected-file
validation (configuration schemas, EditorConfig and 31 changed workflows).

## Changes

- Add rule-specific `nosemgrep` comments to all 62 affected harden-runner references.
  The existing SARIF filter removes accepted inline suppressions before upload;
  unpinned third-party references and other rules retain their findings.
- Report repository-wide backlog counts separately for Dependabot and each code
  scanner. Explain that this is not a count introduced by the push or a count of
  distinct confirmed vulnerabilities. Preserve existing `cq:` state IDs and deduplication.
  Failed API calls do not publish partial totals or record a successful notification.
- Raise the Go requirement and local version pin to **1.26.10** for
  [GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809).
  Revalidate the Windows exception workaround: upstream `runtime/stack.go` and
  `runtime/stack_test.go` are byte-identical between Go 1.26.9 and 1.26.10. Keep the
  source digest check and update its accepted version.
- Update the following six website dependencies through existing pnpm overrides,
  preserving the resolved major versions and leaving unrelated lock entries alone.

| Dependency | Before → after | Dependabot alerts |
| --- | --- | --- |
| body-parser | 1.20.6 → 1.20.9 | [330](https://github.com/cloudposse/atmos/security/dependabot/330) |
| fast-uri | 3.1.8 → 3.1.9 | [331](https://github.com/cloudposse/atmos/security/dependabot/331), [332](https://github.com/cloudposse/atmos/security/dependabot/332), [333](https://github.com/cloudposse/atmos/security/dependabot/333), [334](https://github.com/cloudposse/atmos/security/dependabot/334) |
| follow-redirects | 1.16.0 → 1.16.1 | Original [322](https://github.com/cloudposse/atmos/security/dependabot/322), [323](https://github.com/cloudposse/atmos/security/dependabot/323); current [336](https://github.com/cloudposse/atmos/security/dependabot/336), [337](https://github.com/cloudposse/atmos/security/dependabot/337) |
| serialize-javascript | 7.0.7 → 7.1.3 | [335](https://github.com/cloudposse/atmos/security/dependabot/335) |
| DOMPurify | 3.4.16 → 3.4.17 | [327](https://github.com/cloudposse/atmos/security/dependabot/327), [328](https://github.com/cloudposse/atmos/security/dependabot/328), [329](https://github.com/cloudposse/atmos/security/dependabot/329) |
| launch-editor | 2.14.1 → 2.14.2 | [324](https://github.com/cloudposse/atmos/security/dependabot/324), [325](https://github.com/cloudposse/atmos/security/dependabot/325), [326](https://github.com/cloudposse/atmos/security/dependabot/326) |

The seven-day `minimum-release-age` policy stays enabled. Lockfile generation used
a temporary user npmrc containing `minimum-release-age-exclude[]` entries for only
the six exact patched versions above. A subsequent frozen install succeeds without
that temporary configuration. No persistent package-wide exclusions were added.

The Go patch addresses code-scanning alert
[8504](https://github.com/cloudposse/atmos/security/code-scanning/8504).

## Remaining findings

| Finding | Count | Disposition |
| --- | ---: | --- |
| tinypool [312](https://github.com/cloudposse/atmos/security/dependabot/312), [313](https://github.com/cloudposse/atmos/security/dependabot/313) | 2 | Requires moving 1.1.1 to at least 2.1.2. Major upgrades are excluded by the repository's remediation policy; no forced override. |
| http-cache-semantics [311](https://github.com/cloudposse/atmos/security/dependabot/311) | 1 | Advisory has no patched version. |
| CodeQL path injection, #6273–#6281 | 9 | `FileStore` joins externally supplied cache keys to its root. Needs explicit traversal/symlink threat analysis and a compatible containment design for reads, writes, sidecars and locks; not automatically dismissed or patched by guessing. |
| CodeQL weak hashing [8217](https://github.com/cloudposse/atmos/security/code-scanning/8217), [8581](https://github.com/cloudposse/atmos/security/code-scanning/8581) | 2 | The flagged SHA-256 calls create cache/state identities, not stored password verifiers. Likely false positives; retain pending data-flow review rather than changing the hashing contract. #8581 was not in the original snapshot. |
| govulncheck [8250](https://github.com/cloudposse/atmos/security/code-scanning/8250), [8251](https://github.com/cloudposse/atmos/security/code-scanning/8251), [8252](https://github.com/cloudposse/atmos/security/code-scanning/8252) | 3 | AWS SDK v1 S3 crypto and x/crypto/openpgp advisories have no fixed version. Current scanner messages say vulnerable symbols are not called. Removing the affected dependency paths requires a separate migration. |
| Scorecard [5414](https://github.com/cloudposse/atmos/security/code-scanning/5414) | 1 | Aggregate dependency finding overlaps other scanners. Patching website packages may reduce it, but unpatched Go advisories remain. |
| Trivy [5365](https://github.com/cloudposse/atmos/security/code-scanning/5365) | 1 | Changing the image's default user requires validating bind mounts, managed tool installation and Docker access. No silent change to existing container behavior. |

Path-injection alerts: [6273](https://github.com/cloudposse/atmos/security/code-scanning/6273), [6274](https://github.com/cloudposse/atmos/security/code-scanning/6274), [6275](https://github.com/cloudposse/atmos/security/code-scanning/6275), [6276](https://github.com/cloudposse/atmos/security/code-scanning/6276), [6277](https://github.com/cloudposse/atmos/security/code-scanning/6277), [6278](https://github.com/cloudposse/atmos/security/code-scanning/6278), [6279](https://github.com/cloudposse/atmos/security/code-scanning/6279), [6280](https://github.com/cloudposse/atmos/security/code-scanning/6280), [6281](https://github.com/cloudposse/atmos/security/code-scanning/6281).

## Semgrep alert inventory

All 62 alerts below point to same-repository harden-runner, diagnostics or runtime
references and are covered by the new rule-specific suppressions:

[8297](https://github.com/cloudposse/atmos/security/code-scanning/8297), [8298](https://github.com/cloudposse/atmos/security/code-scanning/8298), [8299](https://github.com/cloudposse/atmos/security/code-scanning/8299), [8300](https://github.com/cloudposse/atmos/security/code-scanning/8300), [8302](https://github.com/cloudposse/atmos/security/code-scanning/8302), [8307](https://github.com/cloudposse/atmos/security/code-scanning/8307), [8311](https://github.com/cloudposse/atmos/security/code-scanning/8311), [8322](https://github.com/cloudposse/atmos/security/code-scanning/8322), [8324](https://github.com/cloudposse/atmos/security/code-scanning/8324), [8328](https://github.com/cloudposse/atmos/security/code-scanning/8328), [8329](https://github.com/cloudposse/atmos/security/code-scanning/8329), [8331](https://github.com/cloudposse/atmos/security/code-scanning/8331), [8332](https://github.com/cloudposse/atmos/security/code-scanning/8332), [8333](https://github.com/cloudposse/atmos/security/code-scanning/8333), [8337](https://github.com/cloudposse/atmos/security/code-scanning/8337), [8338](https://github.com/cloudposse/atmos/security/code-scanning/8338), [8339](https://github.com/cloudposse/atmos/security/code-scanning/8339), [8340](https://github.com/cloudposse/atmos/security/code-scanning/8340), [8351](https://github.com/cloudposse/atmos/security/code-scanning/8351), [8355](https://github.com/cloudposse/atmos/security/code-scanning/8355), [8356](https://github.com/cloudposse/atmos/security/code-scanning/8356), [8357](https://github.com/cloudposse/atmos/security/code-scanning/8357), [8358](https://github.com/cloudposse/atmos/security/code-scanning/8358), [8359](https://github.com/cloudposse/atmos/security/code-scanning/8359), [8360](https://github.com/cloudposse/atmos/security/code-scanning/8360), [8361](https://github.com/cloudposse/atmos/security/code-scanning/8361), [8362](https://github.com/cloudposse/atmos/security/code-scanning/8362), [8363](https://github.com/cloudposse/atmos/security/code-scanning/8363), [8364](https://github.com/cloudposse/atmos/security/code-scanning/8364), [8365](https://github.com/cloudposse/atmos/security/code-scanning/8365), [8366](https://github.com/cloudposse/atmos/security/code-scanning/8366), [8367](https://github.com/cloudposse/atmos/security/code-scanning/8367), [8368](https://github.com/cloudposse/atmos/security/code-scanning/8368), [8369](https://github.com/cloudposse/atmos/security/code-scanning/8369), [8370](https://github.com/cloudposse/atmos/security/code-scanning/8370), [8371](https://github.com/cloudposse/atmos/security/code-scanning/8371), [8372](https://github.com/cloudposse/atmos/security/code-scanning/8372), [8373](https://github.com/cloudposse/atmos/security/code-scanning/8373), [8374](https://github.com/cloudposse/atmos/security/code-scanning/8374), [8375](https://github.com/cloudposse/atmos/security/code-scanning/8375), [8376](https://github.com/cloudposse/atmos/security/code-scanning/8376), [8377](https://github.com/cloudposse/atmos/security/code-scanning/8377), [8378](https://github.com/cloudposse/atmos/security/code-scanning/8378), [8379](https://github.com/cloudposse/atmos/security/code-scanning/8379), [8380](https://github.com/cloudposse/atmos/security/code-scanning/8380), [8381](https://github.com/cloudposse/atmos/security/code-scanning/8381), [8382](https://github.com/cloudposse/atmos/security/code-scanning/8382), [8384](https://github.com/cloudposse/atmos/security/code-scanning/8384), [8385](https://github.com/cloudposse/atmos/security/code-scanning/8385), [8386](https://github.com/cloudposse/atmos/security/code-scanning/8386), [8387](https://github.com/cloudposse/atmos/security/code-scanning/8387), [8388](https://github.com/cloudposse/atmos/security/code-scanning/8388), [8389](https://github.com/cloudposse/atmos/security/code-scanning/8389), [8390](https://github.com/cloudposse/atmos/security/code-scanning/8390), [8391](https://github.com/cloudposse/atmos/security/code-scanning/8391), [8392](https://github.com/cloudposse/atmos/security/code-scanning/8392), [8400](https://github.com/cloudposse/atmos/security/code-scanning/8400), [8401](https://github.com/cloudposse/atmos/security/code-scanning/8401), [8402](https://github.com/cloudposse/atmos/security/code-scanning/8402), [8403](https://github.com/cloudposse/atmos/security/code-scanning/8403), [8404](https://github.com/cloudposse/atmos/security/code-scanning/8404), [8468](https://github.com/cloudposse/atmos/security/code-scanning/8468).

## Validation

- Go 1.26.10: `CGO_ENABLED=0 go build ./...` and targeted tests for
  `pkg/http/proxy` and `internal/ci/vulncheck` pass.
- Eight hook/SARIF tests, eight Windows-workaround tests and 47 workflow-policy/SHA
  verification tests pass. Hook tests cover source counts, empty sources, repeat
  notification suppression, API failures and non-triggering commands.
- The pinned CI Semgrep container scanning `.github` reports zero unsuppressed
  findings. The raw SARIF has 68 accepted inline suppressions (62 newly added plus
  existing suppressions); the existing upload filter leaves zero results. Raw
  findings cover every file represented by the 62 live alerts.
- `pnpm install --frozen-lockfile --ignore-scripts` succeeds with the normal policy.
  All 113 website prebuild tests pass.
- `go tool mage notice:generate` succeeds; `NOTICE` is unchanged.
- Affected-file validation against `HEAD` passes: configuration schemas,
  EditorConfig and all 31 changed workflows. `git diff --check` passes.
- Patch-scoped golangci-lint (`--new-from-rev=HEAD`) reports zero issues.
  `atmos lint --changed` compares against `origin/main` and finds six existing
  issues in untouched Go files: import order in `pkg/hooks/hooks.go`, a heavy
  parameter in `pkg/utils/yaml_tag_walker.go`, complexity in `cmd/secret/set.go`,
  function length in `pkg/auth/manager_chain.go` and `pkg/schema/task.go`, and an
  always-nil error return in `pkg/schema/task.go`.
- The full `atmos test` run is not green. The secret key-generation test discovers
  local default key material and skips creating its fixture key; isolating
  `SOPS_AGE_KEY_FILE` to a nonexistent test path makes the entire `cmd/secret`
  suite pass under Go 1.26.10 with `GOFIPS140=latest`.

`pnpm run build` succeeds, including all 113 prebuild tests and the production
site. It reports dynamic-import, MDX fallback and broken-anchor warnings.
The broad short suite also times out in `github.com/cloudposse/atmos/tests` after
its five-minute limit. The `atmos test` command selects `GOTOOLCHAIN=auto`, which
used the installed Go 1.27.1; the explicit build and targeted tests above used
Go 1.26.10. The full repository check is therefore not a passing release gate.
