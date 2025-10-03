#!/bin/bash
# Script to clean React/MDX import statements from llms.txt files
# These import statements are extracted by docusaurus-plugin-llms from MDX files
# and need to be removed to keep the files as plain text for LLMs

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STATIC_DIR="$SCRIPT_DIR/../static"

echo "Cleaning import statements from llms.txt files..."

# Clean llms.txt - replace import statements with [Description not available]
if [ -f "$STATIC_DIR/llms.txt" ]; then
    sed -i.bak 's/: import [A-Za-z{}, ]* from ['\''"].*['\''"];*$/: [Description not available]/' "$STATIC_DIR/llms.txt"
    rm "$STATIC_DIR/llms.txt.bak"
    echo "✓ Cleaned $STATIC_DIR/llms.txt"
fi

# Clean llms-full.txt - remove import statement lines entirely
if [ -f "$STATIC_DIR/llms-full.txt" ]; then
    sed -i.bak '/^import [A-Za-z{}, ]* from ['\''"].*['\''"];*$/d' "$STATIC_DIR/llms-full.txt"
    rm "$STATIC_DIR/llms-full.txt.bak"
    echo "✓ Cleaned $STATIC_DIR/llms-full.txt"
fi

echo "✓ Import cleanup complete"
