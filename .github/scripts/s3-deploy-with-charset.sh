#!/usr/bin/env bash
#
# Sync a local directory to S3, then force `Content-Type: ...; charset=utf-8`
# on text-format objects.
#
# Why: `aws s3 sync` uses Python's `mimetypes.guess_type`, which never appends
# a charset parameter to the guessed MIME type. Browsers viewing `text/*`
# without a charset fall back to a legacy encoding (Windows-1252/Latin-1),
# corrupting UTF-8 content. Upstream feature request aws/aws-cli#1346 has
# been open since 2015 with no native fix.
#
# Strategy: do the normal sync first (handles uploads + --delete), then
# rewrite the Content-Type of text objects in place with `aws s3 cp
# --metadata-directive REPLACE`. This is a server-side copy (no body
# re-upload) and forces the metadata update regardless of ETag — so it also
# backfills any pre-existing objects whose headers are wrong.
#
# Usage: s3-deploy-with-charset.sh <local_dir> <s3_uri_with_trailing_slash>
#
# Example:
#   s3-deploy-with-charset.sh ./website/build s3://my-bucket/pr-123/

set -euo pipefail

LOCAL_DIR="${1:?local directory required (e.g., ./website/build)}"
S3_URI="${2:?S3 URI required with trailing slash (e.g., s3://my-bucket/pr-123/)}"

# Ensure trailing slash for consistent prefix behavior.
[[ "${S3_URI}" == */ ]] || S3_URI="${S3_URI}/"

echo "::group::Identity"
aws sts get-caller-identity
echo "::endgroup::"

echo "::group::Sync ${LOCAL_DIR} -> ${S3_URI}"
aws s3 sync "${LOCAL_DIR}" "${S3_URI}" --delete
echo "::endgroup::"

# Map text-format extensions to their `Content-Type; charset=utf-8`.
# Server-side copy is cheap, so the list is generous — unmatched extensions
# no-op silently.
declare -A TEXT_TYPES=(
  [html]="text/html; charset=utf-8"
  [htm]="text/html; charset=utf-8"
  [css]="text/css; charset=utf-8"
  [js]="application/javascript; charset=utf-8"
  [mjs]="application/javascript; charset=utf-8"
  [json]="application/json; charset=utf-8"
  [map]="application/json; charset=utf-8"
  [webmanifest]="application/manifest+json; charset=utf-8"
  [xml]="application/xml; charset=utf-8"
  [rss]="application/xml; charset=utf-8"
  [atom]="application/xml; charset=utf-8"
  [svg]="image/svg+xml; charset=utf-8"
  [txt]="text/plain; charset=utf-8"
  [md]="text/markdown; charset=utf-8"
  [csv]="text/plain; charset=utf-8"
  [tsv]="text/plain; charset=utf-8"
  [yaml]="text/plain; charset=utf-8"
  [yml]="text/plain; charset=utf-8"
  [sh]="text/x-shellscript; charset=utf-8"
  [bash]="text/x-shellscript; charset=utf-8"
  [tf]="text/plain; charset=utf-8"
  [tfvars]="text/plain; charset=utf-8"
  [hcl]="text/plain; charset=utf-8"
  [rego]="text/plain; charset=utf-8"
  [toml]="application/toml; charset=utf-8"
  [ini]="text/plain; charset=utf-8"
  [cfg]="text/plain; charset=utf-8"
  [py]="text/plain; charset=utf-8"
  [go]="text/plain; charset=utf-8"
  [rb]="text/plain; charset=utf-8"
  [ts]="text/plain; charset=utf-8"
  [tsx]="text/plain; charset=utf-8"
  [jsx]="text/plain; charset=utf-8"
)

echo "::group::Re-stamp Content-Type with charset=utf-8 for text formats"
for ext in "${!TEXT_TYPES[@]}"; do
  ct="${TEXT_TYPES[$ext]}"
  echo "  *.${ext}  ->  ${ct}"
  aws s3 cp "${S3_URI}" "${S3_URI}" \
    --recursive --exclude "*" --include "*.${ext}" \
    --metadata-directive REPLACE \
    --content-type "${ct}" \
    --no-progress
done
echo "::endgroup::"

echo "::group::Listing"
aws s3 ls "${S3_URI}" --recursive --human-readable --summarize
echo "::endgroup::"
