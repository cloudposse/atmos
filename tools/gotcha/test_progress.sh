#!/bin/bash
echo "Testing progress bar animation with TUI mode..."
echo "Running tests in ./pkg/config..."
echo ""
./gotcha --tui --show=none ./pkg/config &
PID=$!
sleep 3
kill $PID 2>/dev/null
echo ""
echo "Test terminated after 3 seconds"