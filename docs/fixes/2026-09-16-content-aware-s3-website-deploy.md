# Fix: make S3 website deployment content-aware

**Date:** 2026-09-16

## Summary

The production and preview website workflows rewrote large portions of their S3
origins on every deployment, including deployments whose content had not
changed. The deployment now uses a versioned SHA-256 manifest and writes only
added, changed, or removed managed objects. An unchanged deployment performs
one manifest read and zero S3 write requests.

The deployment implementation is the `s3:deploy` Mage target. It replaces the
temporary Python and shell implementation completely, preserves the required
UTF-8 content metadata, and keeps configured out-of-band paths safe from
deletion.

## Context

The old deployment had two independent sources of unnecessary S3 writes:

1. `aws s3 sync` could PUT rebuilt objects because generated mtimes changed,
    even when their bytes did not.
2. A recursive metadata pass copied every matching text object back onto itself
    with `--metadata-directive REPLACE` on every production and preview deploy.

The second operation was needed because the website depends on correct MIME and
charset headers, but it turned a metadata requirement into a recurring full-site
rewrite.

AWS Cost Explorer for the seven complete days from September 9-15, 2026 showed
the scale of the request churn across the development and production
documentation-origin accounts:

| Operation | Requests | Cost |
| --- | ---: | ---: |
| Development `PutObject` | 2,759,667 | $13.7979200 |
| Development `CopyObject` | 2,634,393 | $13.2415158 |
| Production `PutObject` | 547,344 | $2.7366350 |
| Production `CopyObject` | 491,863 | $2.4723017 |
| **Total** | **6,433,267** | **$32.2483725** |

That seven-day total normalizes to approximately **$138.21/month** for
`PutObject` and `CopyObject` alone. All S3 Tier-1 requests were running at
approximately **$145.31/month**. This repository's website deployments are
expected to account for roughly **$70-85/month** of the avoidable run rate.

## Changes

- Added `s3:deploy`, which records each managed path's SHA-256 digest, size, and
  content type in `.cloudposse-deploy-manifest-v1.json`.
- Added deterministic delta calculation. New and changed files are uploaded;
  removed files are deleted in batches; unchanged files receive no S3 write.
- Made manifest publication the final operation. A failed upload or deletion
  leaves the old manifest in place so the next run retries the incomplete delta.
- Use the repository's existing AWS SDK for Go v2 dependency for every S3
  operation. The target no longer shells out to the AWS CLI or writes temporary
  delete-request files.
- Inspect typed `DeleteObjects` responses and reject per-object errors even when
  S3 returns a successful HTTP response.
- Preserve remote paths matched by newline-separated `PROTECTED_PATTERNS`.
  The same local matcher decides which manifest and bootstrap objects to retain.
- Replaced the hand-maintained content-type table with Go's extension MIME
  database plus the repository's existing `github.com/gabriel-vasile/mimetype`
  magic-number detector for unknown extensions. Browser-significant extension
  types remain authoritative, and textual media types receive an explicit
  `charset=utf-8` parameter.
- Changed bootstrap to upload each managed file once with explicit metadata,
  followed by an SDK listing that deletes stale, unprotected remote objects.
  Bootstrap no longer recursively copies S3 objects to restamp them.
- Changed the preview workflow to use the repository's local
  `.github/actions/setup-go-cache` action before invoking Mage.
- Added `magefiles/README.md`, cataloging all exposed Mage targets and their
  behavior-changing environment variables.

## Cost characteristics

| Deployment state | S3 reads | S3 writes |
| --- | --- | --- |
| First deployment | Manifest lookup plus paginated object listing | One PUT/file, stale deletes, and one manifest PUT |
| Unchanged deployment | One manifest GET | Zero |
| Changed deployment | One manifest GET | Only changed/new PUTs, removed-object deletes, and one manifest PUT |

CloudFront invalidation behavior is unchanged. The target optimizes S3 object
writes; it does not weaken cache invalidation or content metadata.

## Validation

- `go test -tags=mage ./magefiles` passes.
- The S3 deployment implementation has **89.9% statement coverage** (232/258).
- Tests cover deterministic content hashing, MIME magic fallback, browser MIME
  overrides, explicit UTF-8 metadata, unchanged zero-write behavior, protected
  paths, rejection of symlinks and other non-regular sources, bootstrap
  pagination, deletion batching, per-object deletion errors, and AWS SDK
  failures.
- `go vet -tags=mage ./magefiles` passes.
- `actionlint .github/workflows/website-preview-deploy.yml
  .github/workflows/website-deploy-prod.yml` passes.
- The repository's changed-file pre-commit suite, including its custom
  golangci-lint build, passes.

## Rollback

Reverting this change restores the recursive sync and metadata-copy behavior.
The manifest object is harmless to older deployments and can be left in place;
if this implementation is reintroduced later, it will use that manifest to
calculate the next delta. Deleting the manifest intentionally forces a safe
one-time bootstrap.

## Follow-ups

After deployment, verify one changed publish and one unchanged repeat publish.
The repeat must log `No content changes; zero S3 writes required.` and show no
`PutObject`, `CopyObject`, or `DeleteObjects` calls for managed website content.
