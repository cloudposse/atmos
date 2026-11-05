# Restore GitHub-generated provenance after the release refactor

RC run [40041249948](https://github.com/cloudposse/atmos/actions/runs/40041249948)
finished building but failed while persisting its attestation: GitHub rejected
`https://atmos.tools/release/v1` as an unsupported SLSA build type. Verification
and publication were skipped; `v1.233.2-rc.1` was not published.

Before refactor #3474, `actions/attest-build-provenance` received only artifact
paths and generated GitHub's supported provenance automatically. The refactor
instead supplied a custom predicate under `https://slsa.dev/provenance/v1` to
distinguish the selected build source from the workflow/tooling revision. The
source distinction is necessary, but that custom SLSA build type is unsupported.

Restore the standard action's default provenance and emit the selected source
as a separate custom predicate with type `https://atmos.tools/release/v1`, using
the existing pinned `actions/attest` action. The verifier requires both signed
statements for every build asset, pins the workflow identity and tooling digest
for both, and checks the custom statement's repository/tag/source SHA. Existing
binary, checksum-signature, and SBOM checks remain in place. The custom schema
is documented in the release-publish action README.

Regression tests prohibit custom predicate inputs on the standard SLSA action,
exercise the actual JavaScript producer against the Go source verifier, and
reject missing or invalid signatures, wrong source identity, and the old SLSA
wrapper. Local fixtures do not exercise GitHub's attestation persistence API;
successful publication still needs validation in the next requested RC.

The old run is pinned to its original workflow revision. After merging the fix,
start a new candidate from the updated main branch rather than rerunning the old
workflow. Leave the failed run's reserved tag intact; preparation assigns the next
available RC number to the new request.
