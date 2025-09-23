#!/bin/bash
set -e

# Extract just terraform commands from demo-stacks.txt and generate screengrabs
grep "^atmos terraform" demo-stacks.txt | while read -r command; do
    echo "Generating screengrab for: $command"
    ./build-all.sh <(echo "$command")
done

echo "All terraform screengrabs generated!"
