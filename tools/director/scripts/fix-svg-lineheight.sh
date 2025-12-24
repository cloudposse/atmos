#!/bin/bash
# Fix SVG line height by adjusting Y coordinates
# Usage: fix-svg-lineheight.sh input.svg output.svg [target_line_height]
#
# VHS generates SVG with ~48px line spacing (charHeight=40, default lineHeight=1.2)
# This script adjusts Y coordinates to achieve proper line height

set -e

INPUT="$1"
OUTPUT="$2"
TARGET_LINE_HEIGHT="${3:-1.2}"  # Default 1.2

if [[ -z "$INPUT" || -z "$OUTPUT" ]]; then
    echo "Usage: $0 input.svg output.svg [target_line_height]"
    exit 1
fi

# Current VHS uses charHeight=40, we want font-size based (14px * 1.2 = 16.8px)
# Scale factor: 16.8 / 48 = 0.35
SCALE=$(echo "scale=4; 14 * $TARGET_LINE_HEIGHT / 48" | bc)

# Process the SVG - adjust all y="N" attributes
sed -E "s/y=\"([0-9]+)\"/y=\"\$(echo \"scale=0; \1 * $SCALE / 1\" | bc)\"/g" "$INPUT" | \
    while IFS= read -r line; do
        # Evaluate the bc expressions
        echo "$line" | sed -E 's/\$\(echo "scale=0; ([0-9]+) \* ([0-9.]+) \/ 1" \| bc\)/\1/g' | \
        awk -v scale="$SCALE" '{
            while (match($0, /y="([0-9]+)"/, arr)) {
                old_y = arr[1]
                new_y = int(old_y * scale)
                gsub(/y="[0-9]+"/, "y=\"" new_y "\"", $0)
            }
            print
        }'
    done > "$OUTPUT"

echo "Fixed SVG line height: $INPUT -> $OUTPUT (scale: $SCALE)"
