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

trap 'rm -rf "${TEMP_DIR}"' EXIT

echo "Generating NOTICE file for Atmos..."
echo "Working directory: ${REPO_ROOT}"
echo "Temporary directory: ${TEMP_DIR}"

cd "${REPO_ROOT}"

# Deterministic license-URL overrides for modules that go-licenses cannot resolve
# reliably. Vanity import paths (e.g. dario.cat/mergo, inet.af/netaddr) live on a
# different host than their module path, so go-licenses must do a network fetch to
# map the path to its source repo — when that fetch fails it emits "URL: Unknown",
# producing a spurious NOTICE diff. Each entry is "<module> <source-repo>"; the URL
# is rebuilt from the module's version in the build list (no network), so it is
# identical on every run regardless of whether go-licenses' resolution succeeded.
# A plain newline-delimited list (not a bash 4 associative array) keeps this working
# on macOS's stock bash 3.2.
REPO_OVERRIDES="
dario.cat/mergo github.com/imdario/mergo
inet.af/netaddr github.com/inetaf/netaddr
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
    local csv="$1" module repo version ref url
    while read -r module repo; do
        [ -n "${module}" ] || continue
        version="$(go list -m -f '{{.Version}}' "${module}" 2>/dev/null || true)"
        [ -n "${version}" ] || continue
        ref="$(git_ref_from_version "${version}")"
        url="https://${repo}/blob/${ref}/LICENSE"
        awk -F',' -v OFS=',' -v mod="${module}" -v newurl="${url}" \
            '$1==mod{$2=newurl} {print}' "${csv}" > "${csv}.tmp" && mv "${csv}.tmp" "${csv}"
    done <<EOF
${REPO_OVERRIDES}
EOF
}

# Check if go-licenses is installed
if ! command -v go-licenses &> /dev/null; then
    echo "Installing go-licenses..."
    go install github.com/google/go-licenses@latest
fi

# Generate license report
echo "Generating license report..."
go-licenses report . 2>&1 | grep -v "^W" | grep -v "^E" > "${TEMP_DIR}/license-report.csv" || true

# Replace non-deterministic (vanity-path) URLs with deterministic overrides.
apply_url_overrides "${TEMP_DIR}/license-report.csv"

# Count dependencies
TOTAL_DEPS=$(wc -l < "${TEMP_DIR}/license-report.csv" | tr -d ' ')
APACHE_DEPS=$(grep -c "Apache-2.0" "${TEMP_DIR}/license-report.csv" || echo "0")
BSD_DEPS=$(grep -cE "BSD-.*Clause" "${TEMP_DIR}/license-report.csv" || echo "0")

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
MPL_COUNT=$(grep -c "MPL-2.0" "${TEMP_DIR}/license-report.csv" || echo "0")
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
MIT_COUNT=$(grep -c ",MIT" "${TEMP_DIR}/license-report.csv" || echo "0")
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
