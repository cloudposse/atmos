//go:build mage

package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/cloudposse/atmos/magefiles/internal/testutil"
	"github.com/stretchr/testify/require"
)

// The helper process records the actual Go command and selected environment,
// then supplies deterministic benchmark output without compiling Atmos.
func TestMain(m *testing.M) {
	if os.Getenv("ATMOS_BENCH_TEST_GO") == "1" {
		os.Exit(benchGoHelper())
	}
	testutil.Main(m)
}

func benchGoHelper() int {
	args := os.Args[1:]
	entry := map[string]any{"args": args}
	for _, key := range []string{"CGO_ENABLED", "GOFLAGS", "GOMAXPROCS", "GOTOOLCHAIN", "ATMOS_BENCH_BINARY", "ATMOS_BENCH_OBSERVE_STORE_READS"} {
		entry[key] = os.Getenv(key)
	}
	log, err := os.OpenFile(os.Getenv("ATMOS_BENCH_TEST_LOG"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return 2
	}
	_ = json.NewEncoder(log).Encode(entry)
	_ = log.Close()
	if len(args) == 0 || args[0] == os.Getenv("ATMOS_BENCH_TEST_FAIL") {
		return 1
	}
	switch args[0] {
	case "env":
		fmt.Println("go1.26.9")
	case "build":
		if len(args) != 4 || args[1] != "-o" {
			return 2
		}
		if err := os.WriteFile(args[2], []byte("fake CLI"), 0o600); err != nil {
			return 2
		}
	case "tool":
		if len(args) != 4 || args[1] != "benchstat" {
			return 2
		}
		fmt.Println("benchstat comparison")
	case "test":
		data, err := os.ReadFile(os.Getenv("ATMOS_BENCH_TEST_DATA"))
		if err != nil {
			return 2
		}
		var packages map[string]string
		if json.Unmarshal(data, &packages) != nil {
			return 2
		}
		fmt.Print(packages[args[1]])
	default:
		return 2
	}
	return 0
}

func benchTestOutput() map[string]string {
	outputs := map[string]string{}
	for _, suite := range benchSuites {
		var out strings.Builder
		fmt.Fprintf(&out, "pkg: %s\n", benchPackage(suite.Package))
		names := suite.Names
		if suite.CLI {
			names = []string{"BenchmarkCLI/DescribeAffected/Dependents/instances=1000"}
			for _, size := range []int{100, 1000, 10000} {
				for _, change := range []string{"none", "catalog", "component"} {
					names = append(names, fmt.Sprintf("BenchmarkCLI/DescribeAffected/instances=%d/change=%s", size, change))
				}
			}
			names = append(names, "BenchmarkCLI/DescribeStacks/instances=1000", "BenchmarkCLI/ListInstances/instances=1000",
				"BenchmarkCLI/DescribeAffected/StoreRepeated/calls=2200/keys=11", "BenchmarkCLI/DescribeStacks/StoreRepeated/calls=2200/keys=11")
		}
		for _, name := range names {
			for range 10 {
				fmt.Fprintf(&out, "%s-2 1 100 ns/op", name)
				if strings.Contains(name, "StoreRepeated/") {
					out.WriteString(" 11 backend-reads/op")
				}
				if !suite.CLI {
					out.WriteString(" 32 B/op 1 allocs/op")
				}
				out.WriteByte('\n')
			}
		}
		outputs[suite.Package] = out.String()
	}
	return outputs
}

func benchSetUpRun(t *testing.T) string {
	t.Helper()
	root := testutil.InitGitRepoFixture(t)
	for _, suite := range benchSuites {
		for _, pattern := range suite.Files {
			path := filepath.Join(root, strings.ReplaceAll(pattern, "*", "fixture"))
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
			require.NoError(t, os.WriteFile(path, []byte("// stable benchmark definition\n"), 0o600))
		}
	}
	t.Chdir(root)
	testutil.SetUpFakePathBinary(t, "go")
	t.Setenv("ATMOS_BENCH_TEST_GO", "1")
	t.Setenv("ATMOS_BENCH_TEST_FAIL", "")
	t.Setenv("ATMOS_BENCH_SAMPLES", "10")
	t.Setenv("ATMOS_BENCH_TEST_LOG", filepath.Join(t.TempDir(), "commands.jsonl"))
	dataPath := filepath.Join(t.TempDir(), "measurements.json")
	require.NoError(t, benchWriteJSON(dataPath, benchTestOutput()))
	t.Setenv("ATMOS_BENCH_TEST_DATA", dataPath)
	return root
}

func TestBenchRunRecordsCompleteSnapshot(t *testing.T) {
	benchSetUpRun(t)
	t.Setenv("ATMOS_BENCH_PR_HEAD_SHA", githubtest.SHA)
	t.Setenv("ATMOS_BENCH_BASE_SHA", githubtest.Base)
	out := filepath.Join(t.TempDir(), "candidate")
	require.NoError(t, (Bench{}).Run(out))
	snapshot, err := benchLoad(out)
	require.NoError(t, err)
	require.True(t, snapshot.Manifest.Complete)
	require.Equal(t, githubtest.SHA, snapshot.Manifest.PRHeadSHA)
	require.Equal(t, githubtest.Base, snapshot.Manifest.BaseSHA)
	require.NotEqual(t, snapshot.Manifest.PRHeadSHA, snapshot.Manifest.SHA, "measured checkout and PR association are distinct")
	require.Len(t, snapshot.Manifest.Fingerprints, len(benchSuites))
	require.Equal(t, "go1.26.9", snapshot.Manifest.GoVersion)
	require.NotEmpty(t, snapshot.Manifest.CPU)
	require.NoFileExists(t, filepath.Join(out, "atmos"))
	require.ErrorContains(t, (Bench{}).Run(out), "already exists")
	data, err := os.ReadFile(os.Getenv("ATMOS_BENCH_TEST_LOG"))
	require.NoError(t, err)
	var calls []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var call map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &call))
		calls = append(calls, call)
	}
	require.Len(t, calls, 2+len(benchSuites))
	for _, call := range calls[1:] {
		require.Equal(t, "0", call["CGO_ENABLED"])
		require.Equal(t, "2", call["GOMAXPROCS"])
		require.Equal(t, "", call["GOFLAGS"])
		require.Equal(t, "false", call["ATMOS_BENCH_OBSERVE_STORE_READS"])
	}
	cli, _ := json.Marshal(calls[2]["args"])
	require.Contains(t, string(cli), "-benchtime=1x")
	require.NotContains(t, string(cli), "-benchmem")
	micro, _ := json.Marshal(calls[3]["args"])
	require.Contains(t, string(micro), "-benchtime=1s")
	require.Contains(t, string(micro), "-benchmem")
}

func TestBenchRunFailuresNeverComplete(t *testing.T) {
	for _, stage := range []string{"env", "build", "test", "samples", "missing measurements"} {
		t.Run(stage, func(t *testing.T) {
			benchSetUpRun(t)
			t.Setenv("ATMOS_BENCH_TEST_FAIL", stage)
			if stage == "samples" {
				t.Setenv("ATMOS_BENCH_SAMPLES", "1")
			}
			if stage == "missing measurements" {
				require.NoError(t, benchWriteJSON(os.Getenv("ATMOS_BENCH_TEST_DATA"), map[string]string{}))
			}
			out := filepath.Join(t.TempDir(), "candidate")
			require.Error(t, (Bench{}).Run(out))
			_, err := benchLoad(out)
			require.Error(t, err)
		})
	}
}

func TestBenchSampleBoundsAndFingerprint(t *testing.T) {
	for _, value := range []string{"", "10", "100", "9", "101", "invalid"} {
		t.Setenv("ATMOS_BENCH_SAMPLES", value)
		n, err := benchSamples()
		if value == "" || value == "10" || value == "100" {
			require.NoError(t, err)
			require.GreaterOrEqual(t, n, 10)
		} else {
			require.Error(t, err)
		}
	}
	root := t.TempDir()
	suite := benchSuite{Package: "./example", Names: []string{"BenchmarkExample"}, Files: []string{"*.go"}}
	_, err := benchFingerprint(root, suite)
	require.ErrorContains(t, err, "source missing")
	source := filepath.Join(root, "fixture.go")
	require.NoError(t, os.WriteFile(source, []byte("first"), 0o600))
	first, err := benchFingerprint(root, suite)
	require.NoError(t, err)
	again, err := benchFingerprint(root, suite)
	require.NoError(t, err)
	require.Equal(t, first, again)
	require.NoError(t, os.WriteFile(source, []byte("changed"), 0o600))
	changed, err := benchFingerprint(root, suite)
	require.NoError(t, err)
	require.NotEqual(t, first, changed)
	suite.Files = []string{"["}
	_, err = benchFingerprint(root, suite)
	require.Error(t, err)
}

func TestBenchRejectsEveryMissingWorkload(t *testing.T) {
	var raw strings.Builder
	for _, output := range benchTestOutput() {
		raw.WriteString(output)
	}
	path := filepath.Join(t.TempDir(), "bench.txt")
	require.NoError(t, os.WriteFile(path, []byte(raw.String()), 0o600))
	values, err := benchReadValues(path)
	require.NoError(t, err)
	require.NoError(t, benchValidateSuite(values))
	for key, samples := range values {
		if !strings.HasSuffix(key, "|sec/op") && !strings.HasSuffix(key, "|backend-reads/op") {
			continue
		}
		if strings.Contains(key, "|CLI/") {
			delete(values, key)
			require.Error(t, benchValidateSuite(values), key)
			values[key] = samples
		}
	}
	require.Error(t, benchValidateSuite(map[string][]float64{}))
	require.ErrorContains(t, benchCompleteManifest(t.TempDir(), &benchManifest{Samples: 10}, map[string][]float64{"missing": {1}}), "expected 10 samples")
}

func TestBenchCollectFileErrors(t *testing.T) {
	root := benchSetUpRun(t)
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "bench.txt"), 0o700))
	require.Error(t, benchCollect(context.Background(), root, dir, &benchManifest{Samples: 10}))
	require.Error(t, benchWriteJSON(filepath.Join(dir, "manifest.json"), make(chan int)))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "blocked"), nil, 0o600))
	require.Error(t, benchPrepareOutput(filepath.Join(dir, "blocked", "child")))
}
