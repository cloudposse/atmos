package benchmarks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A loopback SSM endpoint exercises the real CLI/store/auth pipeline without
// cloud access. Small deterministic latency makes repeated remote reads visible.
func storeFixture(tb testing.TB) (fixture, *atomic.Int64) {
	tb.Helper()
	reads := &atomic.Int64{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Historical releases create an HTTP client per lookup. Let the server
		// close connections so repeated samples cannot exhaust client ports.
		w.Header().Set("Connection", "close")
		if r.Header.Get("X-Amz-Target") != "AmazonSSM.GetParameter" {
			http.Error(w, "unexpected operation", http.StatusBadRequest)
			return
		}
		var request struct{ Name string }
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		reads.Add(1)
		time.Sleep(time.Millisecond)
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_ = json.NewEncoder(w).Encode(map[string]any{"Parameter": map[string]any{"Name": request.Name, "Type": "String", "Value": "offline-store-value", "Version": 1}})
	}))
	tb.Cleanup(server.Close)
	f := makeFixture(tb, 200, "none")
	f.env = []string{"AWS_ACCESS_KEY_ID=benchmark-not-a-real-key", "AWS_SECRET_ACCESS_KEY=benchmark-not-a-real-secret", "AWS_REGION=us-east-1"}
	configPath := filepath.Join(f.root, "atmos.yaml")
	config, err := os.ReadFile(configPath)
	if err != nil {
		tb.Fatal(err)
	}
	writeFixture(tb, configPath, string(config)+fmt.Sprintf("stores:\n  remote:\n    type: aws-ssm-parameter-store\n    options:\n      region: us-east-1\n      endpoint: %s\n", server.URL))
	sharedPath := filepath.Join(f.root, "stacks", "catalog", "shared.yaml")
	shared, err := os.ReadFile(sharedPath)
	if err != nil {
		tb.Fatal(err)
	}
	var lookups strings.Builder
	for key := range 11 {
		fmt.Fprintf(&lookups, "        store_%02d: !template '{{ atmos.Store \"remote\" \"global\" (printf \"target-%%d\" (mod (add (atoi (trimPrefix \"app-\" (default \"app-000\" .vars.name))) %d) 11)) \"id\" }}'\n", key, key)
	}
	writeFixture(tb, sharedPath, strings.Replace(string(shared), "        generation: 1\n", "        generation: 1\n"+lookups.String(), 1))
	fixtureGit(tb, f.root, "add", ".")
	fixtureGit(tb, f.root, "commit", "-qm", "store fixture")
	fixtureGit(tb, f.root, "worktree", "remove", "--force", f.base)
	fixtureGit(tb, f.root, "worktree", "add", "--detach", f.base, "HEAD")
	return f, reads
}

func benchmarkStoreCLI(b *testing.B, binary, command string) {
	b.Helper()
	f, reads := storeFixture(b)
	args, expected := []string{"describe", "stacks"}, 200
	if command == "DescribeAffected" {
		args, expected = []string{"describe", "affected", "--repo-path", f.base}, 0
	}
	var total int64
	b.ResetTimer()
	for range b.N {
		b.StopTimer()
		home := b.TempDir()
		reads.Store(0)
		b.StartTimer()
		data := runCLI(b, binary, f, home, args)
		b.StopTimer()
		validateCLI(b, data, expected, command)
		calls := reads.Load()
		total += calls
		b.Logf("%s backend reads: %d", command, calls)
		// Deferred values are scoped by caller stack and config path: eleven
		// keys in each of two stacks, and both repositories for affected.
		limit := int64(22)
		if command == "DescribeAffected" {
			limit = 44
		}
		if calls < 11 || (os.Getenv("ATMOS_BENCH_OBSERVE_STORE_READS") != "true" && calls > limit) {
			b.Fatalf("store backend reads = %d, want 11..%d", calls, limit)
		}
		b.StartTimer()
	}
	b.ReportMetric(float64(total)/float64(b.N), "backend-reads/op")
}
