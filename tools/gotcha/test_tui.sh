#!/bin/bash
echo "Testing TUI progress bar animation..."
echo "Watch the progress bar below - it should animate from 0% to 100%"
echo ""
# Run gotcha in TUI mode (needs a TTY)
timeout 5 ./gotcha stream --packages="./internal/tui ./internal/parser" --show=none || true
echo ""
echo "Test completed."