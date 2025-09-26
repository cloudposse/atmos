#!/bin/bash
set -e

# Simple version that directly processes commands
export TERM=xterm-256color
export LESS=-X

cd ../../examples/demo-stacks

# Read commands and process them
while IFS= read -r command; do
    # Skip comments
    [[ "$command" =~ ^#.*$ ]] && continue
    [[ -z "$command" ]] && continue

    # Generate output filename
    output_base=$(echo "$command" | sed -E 's/ -/-/g' | sed -E 's/ +/-/g' | sed 's/---/--/g')
    output_file="../../demo/screengrabs/artifacts/${output_base}.txt"

    # Create directory if needed
    output_dir=$(dirname "$output_file")
    mkdir -p "$output_dir"

    echo "Processing: $command"

    # Run the command and capture output
    $command > "$output_file" 2>&1 || true

    echo "  Generated: $output_file"
done < ../../demo/screengrabs/demo-stacks.txt

echo "Done!"
