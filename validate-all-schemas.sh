#!/bin/bash

# validate-all-schemas.sh
# Comprehensive Atmos schema validation script using ajv

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Check for required tools
check_requirements() {
    local missing_tools=()

    if ! command -v ajv &> /dev/null; then
        missing_tools+=("ajv")
    fi

    if ! command -v yq &> /dev/null; then
        if ! command -v python3 &> /dev/null; then
            missing_tools+=("yq or python3")
        fi
    fi

    if [ ${#missing_tools[@]} -gt 0 ]; then
        echo -e "${RED}ERROR: Missing required tools: ${missing_tools[*]}${NC}"
        echo "To install:"
        echo "  ajv: npm install -g ajv-cli"
        echo "  yq:  brew install yq (macOS) or snap install yq (Linux)"
        exit 1
    fi
}

# Counters
TOTAL_CLI=0
PASS_CLI=0
SKIP_CLI=0
FAIL_CLI=0

TOTAL_STACK=0
PASS_STACK=0
SKIP_STACK=0
FAIL_STACK=0

TOTAL_VENDOR=0
PASS_VENDOR=0
SKIP_VENDOR=0
FAIL_VENDOR=0

TOTAL_WORKFLOW=0
PASS_WORKFLOW=0
SKIP_WORKFLOW=0
FAIL_WORKFLOW=0

# Schema paths
CLI_SCHEMA="website/static/schemas/atmos/1.0/cli.json"
STACK_SCHEMA="website/static/schemas/atmos/1.0/stack.json"
VENDOR_SCHEMA="website/static/schemas/atmos/1.0/vendor.json"
WORKFLOW_SCHEMA="website/static/schemas/atmos/1.0/workflow.json"

# Test exclusion patterns - configs with deliberate failures
CLI_EXCLUDE_PATTERNS=(
    "test-cases/validate-type-mismatch"
    "fixtures/scenarios/invalid"
    "fixtures/scenarios/broken"
    "fixtures/scenarios/negative"
    "fixtures/scenarios/schemas-validation-negative"
)

STACK_EXCLUDE_PATTERNS=(
    "test-cases/validate-type-mismatch"
    "fixtures/scenarios/invalid"
    "fixtures/scenarios/broken"
    "fixtures/scenarios/negative"
    "fixtures/scenarios/atmos-stacks-validation/stacks/deploy/nonprod.yaml"
)

VENDOR_EXCLUDE_PATTERNS=(
    "test-cases/validate-type-mismatch"
    "fixtures/scenarios/invalid"
    "fixtures/scenarios/broken"
)

WORKFLOW_EXCLUDE_PATTERNS=(
    "test-cases/validate-type-mismatch"
    "fixtures/scenarios/invalid"
    "workflows/**/test-*"
)

# Function to check if file should be excluded
should_exclude() {
    local file=$1
    shift
    local patterns=("$@")

    for pattern in "${patterns[@]}"; do
        if [[ "$file" == *"$pattern"* ]]; then
            return 0  # Should exclude
        fi
    done
    return 1  # Should not exclude
}

# Function to convert YAML to JSON
yaml_to_json() {
    local yaml_file=$1
    local json_output=""

    # Try yq first (faster)
    if command -v yq &> /dev/null; then
        json_output=$(yq -o json eval '.' "$yaml_file" 2>/dev/null)
    # Fall back to Python
    elif command -v python3 &> /dev/null; then
        json_output=$(python3 -c "
import yaml, json, sys
try:
    with open('$yaml_file', 'r') as f:
        data = yaml.safe_load(f)
        print(json.dumps(data))
except Exception as e:
    sys.exit(1)
" 2>/dev/null)
    fi

    echo "$json_output"
}

# Function to validate JSON against schema
validate_with_ajv() {
    local schema_file=$1
    local json_data=$2
    local temp_json=$(mktemp /tmp/atmos-validate.XXXXXX.json)

    # Write JSON to temp file
    echo "$json_data" > "$temp_json"

    # Validate with ajv (suppress verbose output)
    if ajv validate -s "$schema_file" -d "$temp_json" --spec=draft2020 --strict=false 2>/dev/null; then
        rm -f "$temp_json"
        return 0
    else
        # Get validation errors for debugging (optional)
        # Uncomment to see detailed errors:
        # ajv validate -s "$schema_file" -d "$temp_json" --spec=draft2020 --strict=false 2>&1
        rm -f "$temp_json"
        return 1
    fi
}

# Check requirements first
check_requirements

echo "======================================================="
echo "        Atmos Schema Validation Report"
echo "======================================================="
echo ""
echo -e "${BLUE}Using ajv with JSON Schema Draft 2020-12${NC}"
echo ""

# Part 1: Validate Atmos CLI Configs
echo "=== Validating Atmos CLI Configs (atmos.yaml) ==="
echo ""

# Check if schema exists
if [ ! -f "$CLI_SCHEMA" ]; then
    echo -e "${RED}ERROR: CLI schema not found at $CLI_SCHEMA${NC}"
else
    # Find all atmos.yaml files (excluding directories)
    for config in $(find . -type f \( -name "atmos.yaml" -o -name "atmos.yml" -o -name ".atmos.yaml" -o -name ".atmos.yml" \) 2>/dev/null | sort); do
        ((TOTAL_CLI++))

        # Skip if in exclude list
        if should_exclude "$config" "${CLI_EXCLUDE_PATTERNS[@]}"; then
            echo -e "${YELLOW}⏭️  SKIP${NC} (test): $config"
            ((SKIP_CLI++))
            continue
        fi

        # Convert YAML to JSON
        json_data=$(yaml_to_json "$config")

        if [ -z "$json_data" ]; then
            echo -e "${RED}❌ FAIL${NC} (invalid YAML): $config"
            ((FAIL_CLI++))
            continue
        fi

        # Validate against schema
        if validate_with_ajv "$CLI_SCHEMA" "$json_data"; then
            echo -e "${GREEN}✅ PASS${NC}: $config"
            ((PASS_CLI++))
        else
            echo -e "${RED}❌ FAIL${NC} (schema): $config"
            ((FAIL_CLI++))
        fi
    done
fi

echo ""
echo "CLI Config Summary:"
echo "  Total: $TOTAL_CLI"
echo "  Passed: $PASS_CLI"
echo "  Failed: $FAIL_CLI"
echo "  Skipped: $SKIP_CLI"
echo ""

# Part 2: Validate Stack Manifests
echo "=== Validating Stack Manifests ==="
echo ""

# Check if schema exists
if [ ! -f "$STACK_SCHEMA" ]; then
    echo -e "${RED}ERROR: Stack schema not found at $STACK_SCHEMA${NC}"
else
    # Find all stack YAML files (excluding atmos.yaml)
    for stack in $(find . -path "./stacks/*.yaml" -o -path "./stacks/*.yml" -o -path "./catalog/*.yaml" -o -path "./catalog/*.yml" 2>/dev/null | grep -v "atmos.yaml" | sort); do
        ((TOTAL_STACK++))

        # Skip if in exclude list
        if should_exclude "$stack" "${STACK_EXCLUDE_PATTERNS[@]}"; then
            echo -e "${YELLOW}⏭️  SKIP${NC} (test): $stack"
            ((SKIP_STACK++))
            continue
        fi

        # Convert YAML to JSON
        json_data=$(yaml_to_json "$stack")

        if [ -z "$json_data" ]; then
            echo -e "${RED}❌ FAIL${NC} (invalid YAML): $stack"
            ((FAIL_STACK++))
            continue
        fi

        # Validate against schema
        if validate_with_ajv "$STACK_SCHEMA" "$json_data"; then
            echo -e "${GREEN}✅ PASS${NC}: $stack"
            ((PASS_STACK++))
        else
            echo -e "${RED}❌ FAIL${NC} (schema): $stack"
            ((FAIL_STACK++))
        fi
    done
fi

echo ""
echo "Stack Manifest Summary:"
echo "  Total: $TOTAL_STACK"
echo "  Passed: $PASS_STACK"
echo "  Failed: $FAIL_STACK"
echo "  Skipped: $SKIP_STACK"
echo ""

# Part 3: Validate Vendor Configs
echo "=== Validating Vendor Configs ==="
echo ""

# Check if schema exists
if [ ! -f "$VENDOR_SCHEMA" ]; then
    echo -e "${RED}ERROR: Vendor schema not found at $VENDOR_SCHEMA${NC}"
else
    for vendor in $(find . -type f \( -name "vendor.yaml" -o -name "vendor.yml" \) 2>/dev/null | sort); do
        ((TOTAL_VENDOR++))

        # Skip if in exclude list
        if should_exclude "$vendor" "${VENDOR_EXCLUDE_PATTERNS[@]}"; then
            echo -e "${YELLOW}⏭️  SKIP${NC} (test): $vendor"
            ((SKIP_VENDOR++))
            continue
        fi

        # Convert YAML to JSON
        json_data=$(yaml_to_json "$vendor")

        if [ -z "$json_data" ]; then
            echo -e "${RED}❌ FAIL${NC} (invalid YAML): $vendor"
            ((FAIL_VENDOR++))
            continue
        fi

        # Validate against schema
        if validate_with_ajv "$VENDOR_SCHEMA" "$json_data"; then
            echo -e "${GREEN}✅ PASS${NC}: $vendor"
            ((PASS_VENDOR++))
        else
            echo -e "${RED}❌ FAIL${NC} (schema): $vendor"
            ((FAIL_VENDOR++))
        fi
    done
fi

echo ""
echo "Vendor Config Summary:"
echo "  Total: $TOTAL_VENDOR"
echo "  Passed: $PASS_VENDOR"
echo "  Failed: $FAIL_VENDOR"
echo "  Skipped: $SKIP_VENDOR"
echo ""

# Part 4: Validate Workflow Configs
echo "=== Validating Workflow Configs ==="
echo ""

# Check if schema exists
if [ ! -f "$WORKFLOW_SCHEMA" ]; then
    echo -e "${YELLOW}WARNING: Workflow schema not found at $WORKFLOW_SCHEMA${NC}"
    echo "  Skipping workflow validation"
else
    for workflow in $(find . -path "*/workflows/*.yaml" -o -path "*/workflows/*.yml" 2>/dev/null | sort); do
        ((TOTAL_WORKFLOW++))

        # Skip if in exclude list
        if should_exclude "$workflow" "${WORKFLOW_EXCLUDE_PATTERNS[@]}"; then
            echo -e "${YELLOW}⏭️  SKIP${NC} (test): $workflow"
            ((SKIP_WORKFLOW++))
            continue
        fi

        # Convert YAML to JSON
        json_data=$(yaml_to_json "$workflow")

        if [ -z "$json_data" ]; then
            echo -e "${RED}❌ FAIL${NC} (invalid YAML): $workflow"
            ((FAIL_WORKFLOW++))
            continue
        fi

        # Validate against schema
        if validate_with_ajv "$WORKFLOW_SCHEMA" "$json_data"; then
            echo -e "${GREEN}✅ PASS${NC}: $workflow"
            ((PASS_WORKFLOW++))
        else
            echo -e "${RED}❌ FAIL${NC} (schema): $workflow"
            ((FAIL_WORKFLOW++))
        fi
    done
fi

echo ""
echo "Workflow Config Summary:"
echo "  Total: $TOTAL_WORKFLOW"
echo "  Passed: $PASS_WORKFLOW"
echo "  Failed: $FAIL_WORKFLOW"
echo "  Skipped: $SKIP_WORKFLOW"
echo ""

# Calculate totals
TOTAL=$((TOTAL_CLI + TOTAL_STACK + TOTAL_VENDOR + TOTAL_WORKFLOW))
PASSED=$((PASS_CLI + PASS_STACK + PASS_VENDOR + PASS_WORKFLOW))
FAILED=$((FAIL_CLI + FAIL_STACK + FAIL_VENDOR + FAIL_WORKFLOW))
SKIPPED=$((SKIP_CLI + SKIP_STACK + SKIP_VENDOR + SKIP_WORKFLOW))

echo "======================================================="
echo "                  Overall Summary"
echo "======================================================="
echo ""
echo "Total Files Checked: $TOTAL"
echo "  ✅ Passed: $PASSED"
echo "  ❌ Failed: $FAILED"
echo "  ⏭️  Skipped: $SKIPPED (test files with known issues)"
echo ""

if [ $FAILED -gt 0 ]; then
    echo -e "${RED}⚠️  $FAILED config file(s) failed validation${NC}"
    echo ""
    echo "Note: Failed files should be investigated to determine if they need:"
    echo "  1. Schema updates to support their patterns"
    echo "  2. Config fixes to match the schema"
    echo "  3. Addition to the exclusion list if they're test files"
    exit 1
else
    echo -e "${GREEN}✅ All config files passed validation!${NC}"
    exit 0
fi
