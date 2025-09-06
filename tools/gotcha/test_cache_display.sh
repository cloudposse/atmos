#\!/bin/bash
echo "=== Testing Cached Test Count Display ==="
echo ""
echo "Current cache for ./... pattern:"
grep -A2 "./...:$" .gotcha/cache.yaml | grep count | head -1
echo ""
echo "Running gotcha with a small test subset to check if estimate shows..."
echo "(The TUI should show '530 tests (est.)' initially if cache is working)"
echo ""
./gotcha-binary ./pkg/cache --show=none 2>&1 | head -20
