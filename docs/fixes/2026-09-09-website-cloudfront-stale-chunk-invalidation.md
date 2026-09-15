# Fix: website deploys now invalidate CloudFront so stale `index.html` can't 404 on deleted chunks

**Date:** 2026-09-09

## Summary

atmos.tools threw a browser `ChunkLoadError` (404 on a hashed `assets/js/*.js`
bundle referenced by the already-loaded runtime). This looked like a repeat of
the 2026-09-08 concurrent-deploy race
(`docs/fixes/2026-09-08-website-deploy-race-serialize.md`), but that fix is
working correctly — GitHub Actions run history shows the second of two
`Website Deploy Prod` runs queued behind the first via the `concurrency` group
and only started its job after the first completed. The real gap:
`website-deploy-prod.yml` runs `aws s3 sync --delete` against the S3 origin but
never invalidates the CloudFront distribution in front of it, so an edge
location caching the prior deploy's `index.html` can keep serving it (and its
now-deleted chunk references) for up to `cloudfront_min_ttl` after a newer
deploy's `--delete` pass removes those chunks.

## Context

The `atmos-docs-site-spa` distributions (`cloudposse/infra-live`,
`stacks/catalog/spa-s3-cloudfront/{defaults,atmos-docs/defaults}.yaml`) use the
legacy per-behavior TTL fields, not a modern cache policy: `cloudfront_min_ttl:
60`, `cloudfront_default_ttl: 60`. CloudFront's legacy TTL model uses `min_ttl`
as an absolute floor regardless of the origin's `Cache-Control` header, so even
adding explicit no-cache headers to `index.html` in `deploy.sh` would not have
closed the gap — the fix has to be an invalidation, not a header change.

Confirmed (read-only, via `gh api`/`gh pr view` against `cloudposse/infra-live`)
that this floor is unrelated to `cloudposse/infra-live#1706`, a same-day PR the
reporter suspected — that PR only added a CORS viewer-response CloudFront
Function scoped to the `/img/*` behavior and explicitly keeps "Production ...
existing TTLs" on the default behavior unchanged.

The fix is fully containable in `cloudposse/atmos`: `infra-live`'s
`components/terraform/spa-s3-cloudfront/github-actions-iam-policy.tf` already
grants both deploy roles (`cplive-plat-ue2-prod-atmos-docs-gha` and
`cplive-plat-ue2-dev-atmos-docs-gha`) `cloudfront:CreateInvalidation` /
`cloudfront:GetInvalidation` scoped to their own distribution ARN. That
permission was simply unused. No `infra-live` change was needed.

## Changes

- `.github/actions/s3-deploy/action.yml`: added optional
  `cloudfront-distribution-id` / `cloudfront-invalidation-paths` inputs and a
  composite step that runs `aws cloudfront create-invalidation` when a
  distribution ID is supplied. Fire-and-forget (no `wait
  invalidation-completed`) — invalidation typically completes in well under a
  minute and blocking would only slow the deploy job.
- `.github/workflows/website-deploy-prod.yml`: added `CLOUDFRONT_DISTRIBUTION_ID:
  E2WVYUGW40ZPA8` (prod `atmos-docs-site-spa`) and passed it to the `Copy
  Website to S3 Bucket` step with the default `/*` invalidation path. Added
  `cloudfront.amazonaws.com:443` to the Harden Runner allowlist (CloudFront's
  control-plane API is a single global endpoint, not regional).
- `.github/workflows/website-preview-deploy.yml`: same pattern for the dev
  distribution (`E1U29O8X09M5UR`), scoped to `/pr-<N>/*` instead of `/*` so one
  PR's deploy never invalidates every other open PR's cached preview.

## Validation

- `actionlint` on both modified workflow files: clean.
- `python3 -c "import yaml; yaml.safe_load(open(...))"` on the modified
  `s3-deploy/action.yml`: parses. (`actionlint` itself doesn't understand
  composite `action.yml` files — confirmed pre-existing across every action in
  `.github/actions/`, not specific to this change.)
- Confirmed the exact `aws cloudfront create-invalidation --distribution-id
  E2WVYUGW40ZPA8 --paths '/*'` command the new step runs actually succeeds under
  the deploy role's permissions: ran it manually via `atmos auth exec` against
  `cloudposse/infra-live` (`--profile managers --identity plat-prod/terraform`)
  as an immediate remediation for the live incident. Invalidation
  `IB9LFMBACSW7CEWQ9MRS7ZOPRZ` reached `Completed` within about a minute.
- Confirmed via `aws iam get-role-policy` (not just the Terraform source) that
  the *deployed* `cplive-plat-ue2-prod-atmos-docs-gha` role policy already
  contains the `CloudfrontActions` statement (`cloudfront:CreateInvalidation`,
  `cloudfront:GetInvalidation` on `arn:aws:cloudfront::557075604627:distribution/E2WVYUGW40ZPA8`),
  so the new CI step will work under the real OIDC-assumed role with no
  `infra-live` change required.
- Still pending: watching an actual `Website Deploy Prod` run (post-merge)
  execute the new `Invalidate CloudFront Cache` step end-to-end in CI.

## Follow-ups

None.
