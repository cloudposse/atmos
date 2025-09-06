#\!/bin/bash
echo "Testing TUI with cached count..."
echo "Cache file shows test count:"
grep "count:" .gotcha/cache.yaml | head -1

echo ""
echo "Running gotcha (Press Ctrl+C after 2 seconds)..."
./gotcha-binary --show=none ./pkg/utils 2>&1 &
PID=$\!
sleep 2
kill -INT $PID 2>/dev/null
wait $PID 2>/dev/null

echo ""
echo "Cache should not be updated after abort. Checking..."
grep "count:" .gotcha/cache.yaml | head -1
