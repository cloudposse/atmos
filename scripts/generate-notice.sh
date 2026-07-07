#!/usr/bin/env bash
#
# Generate NOTICE file from Go dependencies
# Uses go-licenses to extract license and copyright information
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
NOTICE_FILE="${REPO_ROOT}/NOTICE"
TEMP_DIR=$(mktemp -d)
GO_LICENSES_BIN="$(command -v go-licenses || true)"
GO_LICENSES_VERSION="${GO_LICENSES_VERSION:-v1.6.0}"
LICENSE_GOOS="${LICENSE_GOOS:-linux}"
LICENSE_GOARCH="${LICENSE_GOARCH:-amd64}"
LICENSE_CGO_ENABLED="${LICENSE_CGO_ENABLED:-1}"

trap 'rm -rf "${TEMP_DIR}"' EXIT

echo "Generating NOTICE file for Atmos..."
echo "Working directory: ${REPO_ROOT}"
echo "Temporary directory: ${TEMP_DIR}"
echo "License target: GOOS=${LICENSE_GOOS} GOARCH=${LICENSE_GOARCH} CGO_ENABLED=${LICENSE_CGO_ENABLED}"

cd "${REPO_ROOT}"

GO_BIN="$(go env GOPATH)/bin"
export PATH="${GO_BIN}:${PATH}"

# Deterministic license-URL overrides for modules that go-licenses cannot resolve
# reliably. Vanity import paths and split-module repos sometimes require
# network/source metadata lookups; when those fail, go-licenses emits
# "URL: Unknown", producing a spurious NOTICE diff. Each entry is pipe-delimited:
# "<module>|<source-repo>|<tag-prefix>|<license-path>". The URL is rebuilt from
# the module's version in the build list (no network), so it is identical on every
# run regardless of whether go-licenses' resolution succeeded.
# A plain newline-delimited list (not a bash 4 associative array) keeps this working
# on macOS's stock bash 3.2.
REPO_OVERRIDES="
dario.cat/mergo|github.com/imdario/mergo||LICENSE
inet.af/netaddr|github.com/inetaf/netaddr||LICENSE
go4.org/intern|github.com/go4org/intern||LICENSE
go4.org/netipx|github.com/go4org/netipx||LICENSE
go4.org/unsafe/assume-no-moving-gc|github.com/go4org/unsafe-assume-no-moving-gc||LICENSE
cloud.google.com/go|github.com/googleapis/google-cloud-go||LICENSE
cloud.google.com/go/auth|github.com/googleapis/google-cloud-go|auth|auth/LICENSE
cloud.google.com/go/auth/oauth2adapt|github.com/googleapis/google-cloud-go|auth/oauth2adapt|auth/oauth2adapt/LICENSE
cloud.google.com/go/compute/metadata|github.com/googleapis/google-cloud-go|compute/metadata|compute/metadata/LICENSE
cloud.google.com/go/iam|github.com/googleapis/google-cloud-go|iam|iam/LICENSE
cloud.google.com/go/kms|github.com/googleapis/google-cloud-go|kms|kms/LICENSE
cloud.google.com/go/longrunning|github.com/googleapis/google-cloud-go|longrunning|longrunning/LICENSE
cloud.google.com/go/monitoring|github.com/googleapis/google-cloud-go|monitoring|monitoring/LICENSE
cloud.google.com/go/secretmanager|github.com/googleapis/google-cloud-go|secretmanager|secretmanager/LICENSE
cloud.google.com/go/storage|github.com/googleapis/google-cloud-go|storage|storage/LICENSE
"

# git_ref_from_version maps a module version to a ref usable in a GitHub blob URL:
# a tag (e.g. v1.0.2) is used verbatim; a pseudo-version (v0.0.0-<ts>-<commit>)
# resolves to its trailing commit hash (the full pseudo-version is not a git ref).
git_ref_from_version() {
    local version="$1"
    if [[ "${version}" =~ -([0-9a-f]{12})$ ]]; then
        printf '%s' "${BASH_REMATCH[1]}"
    else
        printf '%s' "${version}"
    fi
}

# apply_url_overrides rewrites the URL (2nd CSV field) for each overridden module to
# a deterministic, version-pinned LICENSE URL derived from go.mod (no network fetch).
apply_url_overrides() {
    local csv="$1" module repo ref_prefix license_path version base_ref ref url
    while IFS='|' read -r module repo ref_prefix license_path; do
        [ -n "${module}" ] || continue
        version="$(GOOS="${LICENSE_GOOS}" GOARCH="${LICENSE_GOARCH}" CGO_ENABLED="${LICENSE_CGO_ENABLED}" go list -m -f '{{.Version}}' "${module}" 2>/dev/null || true)"
        [ -n "${version}" ] || continue
        base_ref="$(git_ref_from_version "${version}")"
        if [ -n "${ref_prefix}" ]; then
            ref="${ref_prefix}/${base_ref}"
        else
            ref="${base_ref}"
        fi
        url="https://${repo}/blob/${ref}/${license_path}"
        awk -F',' -v OFS=',' -v mod="${module}" -v newurl="${url}" \
            '$1==mod{$2=newurl} {print}' "${csv}" > "${csv}.tmp" && mv "${csv}.tmp" "${csv}"
    done <<EOF
${REPO_OVERRIDES}
EOF
}

# Check if go-licenses is installed
if [ -z "${GO_LICENSES_BIN}" ]; then
    echo "Installing go-licenses ${GO_LICENSES_VERSION}..."
    go install "github.com/google/go-licenses@${GO_LICENSES_VERSION}"
    GOBIN="$(go env GOBIN)"
    if [ -z "${GOBIN}" ]; then
        GOBIN="$(go env GOPATH)/bin"
    fi
    GO_LICENSES_BIN="${GOBIN}/go-licenses"
fi

# Generate license report
echo "Generating license report..."
GOOS="${LICENSE_GOOS}" GOARCH="${LICENSE_GOARCH}" CGO_ENABLED="${LICENSE_CGO_ENABLED}" "${GO_LICENSES_BIN}" report . 2>&1 | grep -v "^W" | grep -v "^E" > "${TEMP_DIR}/license-report.csv" || true

# Replace non-deterministic (vanity-path) URLs with deterministic overrides.
apply_url_overrides "${TEMP_DIR}/license-report.csv"

# Count dependencies
TOTAL_DEPS=$(wc -l < "${TEMP_DIR}/license-report.csv" | tr -d ' ')
APACHE_DEPS=$(grep -c "Apache-2.0" "${TEMP_DIR}/license-report.csv" || true)
BSD_DEPS=$(grep -cE "BSD-.*Clause" "${TEMP_DIR}/license-report.csv" || true)

echo "Found ${TOTAL_DEPS} total dependencies"
echo "  - ${APACHE_DEPS} Apache-2.0 licenses"
echo "  - ${BSD_DEPS} BSD licenses"

# Generate NOTICE file header
cat > "${NOTICE_FILE}" <<'EOF'
NOTICE

Atmos - Universal Tool for DevOps and Cloud Automation
Copyright 2021-2025 Cloud Posse, LLC

This product includes software developed by Cloud Posse, LLC and the Atmos community.

================================================================================

This product bundles the following dependencies under their respective licenses.
The license information for each dependency can be found below.

For the full license texts, see the LICENSE file in each dependency or visit
the URLs listed below.

================================================================================

APACHE 2.0 LICENSED DEPENDENCIES
EOF

echo "" >> "${NOTICE_FILE}"

# Add Apache-2.0 dependencies
grep "Apache-2.0" "${TEMP_DIR}/license-report.csv" | \
    sort | \
    awk -F',' '{printf "  - %s\n    License: Apache-2.0\n    URL: %s\n\n", $1, $2}' >> "${NOTICE_FILE}"

# Add BSD section
cat >> "${NOTICE_FILE}" <<'EOF'

================================================================================

BSD LICENSED DEPENDENCIES
EOF

echo "" >> "${NOTICE_FILE}"

# Add BSD dependencies
grep -E "BSD-.*Clause" "${TEMP_DIR}/license-report.csv" | \
    sort | \
    awk -F',' '{printf "  - %s\n    License: %s\n    URL: %s\n\n", $1, $3, $2}' >> "${NOTICE_FILE}"

# Add MPL section if there are any
MPL_COUNT=$(grep -c "MPL-2.0" "${TEMP_DIR}/license-report.csv" || true)
if [ "${MPL_COUNT}" -gt 0 ]; then
    cat >> "${NOTICE_FILE}" <<'EOF'

================================================================================

MOZILLA PUBLIC LICENSE (MPL) 2.0 DEPENDENCIES
EOF

    echo "" >> "${NOTICE_FILE}"

    grep "MPL-2.0" "${TEMP_DIR}/license-report.csv" | \
        sort | \
        awk -F',' '{printf "  - %s\n    License: MPL-2.0\n    URL: %s\n\n", $1, $2}' >> "${NOTICE_FILE}"
fi

# Add MIT section (optional, for completeness)
MIT_COUNT=$(grep -c ",MIT" "${TEMP_DIR}/license-report.csv" || true)
if [ "${MIT_COUNT}" -gt 0 ]; then
    cat >> "${NOTICE_FILE}" <<'EOF'

================================================================================

MIT LICENSED DEPENDENCIES
EOF

    echo "" >> "${NOTICE_FILE}"

    grep ",MIT" "${TEMP_DIR}/license-report.csv" | \
        sort | \
        awk -F',' '{printf "  - %s\n    License: MIT\n    URL: %s\n\n", $1, $2}' >> "${NOTICE_FILE}"
fi

# Add footer
cat >> "${NOTICE_FILE}" <<'EOF'

================================================================================

For the complete list of dependencies and their licenses, run:
  go-licenses report .

To view the full license text for a specific dependency, visit the URL
listed above or check the dependency's repository.

For more information about Atmos licensing, see:
  https://github.com/cloudposse/atmos
EOF

echo "✅ NOTICE file generated successfully: ${NOTICE_FILE}"
echo ""
echo "Summary:"
echo "  - Total dependencies: ${TOTAL_DEPS}"
echo "  - Apache-2.0: ${APACHE_DEPS}"
echo "  - BSD: ${BSD_DEPS}"
if [ "${MPL_COUNT}" -gt 0 ]; then
    echo "  - MPL-2.0: ${MPL_COUNT}"
fi
if [ "${MIT_COUNT}" -gt 0 ]; then
    echo "  - MIT: ${MIT_COUNT}"
fi
echo ""
echo "Review the NOTICE file and commit it to the repository."
