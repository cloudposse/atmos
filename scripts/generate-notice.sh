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

# Check if go-licenses is installed
if ! command -v go-licenses &> /dev/null; then
    echo "Installing go-licenses..."
    go install github.com/google/go-licenses@latest
fi

# Generate license report
echo "Generating license report..."
go-licenses report . 2>&1 | grep -v "^W" | grep -v "^E" > "${TEMP_DIR}/license-report.csv" || true

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
