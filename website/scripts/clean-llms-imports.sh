#!/bin/bash
# Script to clean React/MDX import statements from llms.txt files
# and fix blog URLs to match Docusaurus routing configuration
#
# These import statements are extracted by docusaurus-plugin-llms from MDX files
# and need to be removed to keep the files as plain text for LLMs
#
# WORKAROUND FOR PLUGIN BUG:
# Blog URLs need to be fixed because docusaurus-plugin-llms v0.2.2 has a bug where it:
# 1. Uses filename (2025-10-13-introducing-atmos-auth.md) instead of frontmatter 'slug'
# 2. Uses hardcoded 'blog' path instead of respecting blog.routeBasePath config
# 3. Only removes single numeric prefixes (01-) not full dates (2025-10-13-)
#
# The plugin's route matching tries:
#   /blog/10-13-introducing-atmos-auth  (only removed '2025-')
# But Docusaurus actually routes to:
#   /changelog/introducing-atmos-auth   (using frontmatter slug + routeBasePath)
#
# This workaround should be removed once the plugin is fixed upstream.
# Related issue: https://github.com/rachfop/docusaurus-plugin-llms/issues/15
# (The same fundamental problem with URL path reconstruction, but for blog date prefixes)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STATIC_DIR="$SCRIPT_DIR/../static"

echo "Cleaning import statements and fixing blog URLs in llms.txt files..."

# Clean llms.txt - replace import statements with [Description not available]
if [ -f "$STATIC_DIR/llms.txt" ]; then
    sed -i.bak 's/: import [A-Za-z{}, ]* from ['\''"].*['\''"];*$/: [Description not available]/' "$STATIC_DIR/llms.txt"

    # Fix blog URLs: change /blog/ to /changelog/ and remove date prefixes
    # Handles both dated posts (2025-10-13-slug) and non-dated posts (welcome)
    # Pattern 1: https://atmos.tools/blog/2025-10-13-introducing-atmos-auth -> https://atmos.tools/changelog/introducing-atmos-auth
    sed -i.bak 's|https://atmos\.tools/blog/[0-9]\{4\}-[0-9]\{2\}-[0-9]\{2\}-\([^):]*\)|https://atmos.tools/changelog/\1|g' "$STATIC_DIR/llms.txt"
    # Pattern 2: https://atmos.tools/blog/welcome -> https://atmos.tools/changelog/welcome
    sed -i.bak 's|https://atmos\.tools/blog/\([^):]*\)|https://atmos.tools/changelog/\1|g' "$STATIC_DIR/llms.txt"

    rm "$STATIC_DIR/llms.txt.bak"
    echo "✓ Cleaned $STATIC_DIR/llms.txt"
fi

# Clean llms-full.txt - remove import statement lines entirely
if [ -f "$STATIC_DIR/llms-full.txt" ]; then
    sed -i.bak '/^import [A-Za-z{}, ]* from ['\''"].*['\''"];*$/d' "$STATIC_DIR/llms-full.txt"

    # Fix blog URLs: change /blog/ to /changelog/ and remove date prefixes
    # Handles both dated posts (2025-10-13-slug) and non-dated posts (welcome)
    sed -i.bak 's|https://atmos\.tools/blog/[0-9]\{4\}-[0-9]\{2\}-[0-9]\{2\}-\([^):]*\)|https://atmos.tools/changelog/\1|g' "$STATIC_DIR/llms-full.txt"
    sed -i.bak 's|https://atmos\.tools/blog/\([^):]*\)|https://atmos.tools/changelog/\1|g' "$STATIC_DIR/llms-full.txt"

    rm "$STATIC_DIR/llms-full.txt.bak"
    echo "✓ Cleaned $STATIC_DIR/llms-full.txt"
fi

echo "✓ Import cleanup and blog URL fixing complete"
