#!/bin/bash
# Check CLAUDE.md and agent file sizes to ensure they stay under the limit.

set -e

MAX_SIZE=40000  # 40KB limit for CLAUDE.md files
AGENT_MAX_SIZE=25000  # 25KB limit for agent files
EXIT_CODE=0

# Check all CLAUDE.md files in the repository.
for file in CLAUDE.md .conductor/*/CLAUDE.md; do
    if [ ! -f "$file" ]; then
        continue
    fi

    size=$(wc -c < "$file" | tr -d ' ')

    if [ "$size" -gt "$MAX_SIZE" ]; then
        over=$((size - MAX_SIZE))
        percent=$((over * 100 / MAX_SIZE))

        echo "❌ CLAUDE.md Too Large"
        echo "The modified $file exceeds the $MAX_SIZE byte size limit:"
        echo ""
        echo "$file: $size bytes (over by $over bytes, ~${percent}%)"
        echo ""
        echo "Action needed: Please refactor the oversized CLAUDE.md file. Consider:"
        echo ""
        echo "  • Removing verbose explanations"
        echo "  • Consolidating redundant examples"
        echo "  • Keeping only essential requirements"
        echo "  • Moving detailed guides to separate docs in docs/ or docs/prd/"
        echo ""
        echo "All MANDATORY requirements must be preserved."
        echo ""
        EXIT_CODE=1
    fi
done

# Check all agent files in .claude/agents/.
for file in .claude/agents/*.md; do
    if [ ! -f "$file" ]; then
        continue
    fi

    size=$(wc -c < "$file" | tr -d ' ')

    if [ "$size" -gt "$AGENT_MAX_SIZE" ]; then
        over=$((size - AGENT_MAX_SIZE))
        percent=$((over * 100 / AGENT_MAX_SIZE))

        echo "❌ Agent File Too Large"
        echo "The modified $file exceeds the $AGENT_MAX_SIZE byte size limit:"
        echo ""
        echo "$file: $size bytes (over by $over bytes, ~${percent}%)"
        echo ""
        echo "Action needed: Please refactor the oversized agent file. Consider:"
        echo ""
        echo "  • Removing verbose explanations and examples"
        echo "  • Consolidating redundant sections"
        echo "  • Keeping only essential instructions"
        echo "  • Moving detailed documentation to docs/prd/"
        echo ""
        echo "Agent files should be concise and focused on specific tasks."
        echo ""
        EXIT_CODE=1
    fi
done

exit $EXIT_CODE
