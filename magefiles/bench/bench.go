// Package bench collects and compares reproducible performance snapshots.
package bench

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/magefile/mage/mg"
)

// Bench collects and compares reproducible performance snapshots.
type Bench mg.Namespace

const (
	benchSmallInstances   = 100
	benchMediumInstances  = 1000
	benchLargeInstances   = 10000
	benchSchema           = 1
	benchMinSamples       = 10
	benchMaxSamples       = 100
	benchHistoryLimit     = 10
	benchDefaultThreshold = 10.0
	benchAlpha            = 0.05
	benchConfidence       = 0.95
	benchPercent          = 100.0
	PrivateFile           = 0o600
	benchPrivateDir       = 0o700
	Decimal               = 10
	FloatBits             = 64
	benchHistoryPages     = 10
	CommentLimit          = 55000
	benchPerformanceLimit = 30000
	Workflow              = "benchmarks.yml"
	Marker                = "<!-- atmos-benchmarks -->"
)

var errBenchmark = errors.New("benchmark")

func Error(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errBenchmark, fmt.Sprintf(format, args...))
}

type benchSuite struct {
	Package string
	Names   []string
	Files   []string
	CLI     bool
}

var benchSuites = []benchSuite{
	{"./tests/benchmarks", []string{"BenchmarkCLI"}, []string{"tests/benchmarks/*.go"}, true},
	{"./pkg/merge", []string{"BenchmarkDeepCopyMap", "BenchmarkDeepCopyMap_Large", "BenchmarkMergeNative_TwoInputs", "BenchmarkMergeNative_FiveInputs", "BenchmarkMerge_ProductionScale"}, []string{"pkg/merge/merge_test.go", "pkg/merge/merge_native_test.go"}, false},
	{"./internal/exec", []string{"BenchmarkFindAffectedParallel", "BenchmarkTemplateProcessingComplex", "BenchmarkResolutionContextPushPop", "BenchmarkDeferredStoreRepeated", "BenchmarkLocalsRepeated"}, []string{"internal/exec/describe_affected_benchmark*_test.go", "internal/exec/template_processing_test.go", "internal/exec/yaml_func_resolution_context_bench_test.go", "internal/exec/stack_processor_utils_test.go", "internal/exec/template_funcs_store_deferred_test.go"}, false},
	{"./pkg/list", []string{"BenchmarkCollectInstances", "BenchmarkSortInstances"}, []string{"pkg/list/list_instances_bench_test.go"}, false},
}

type benchManifest struct {
	Mode, SourceRef, HarnessSHA                             string
	Schema                                                  int
	SHA, Repository, RunID, Attempt, Event, Branch, BaseSHA string
	Started                                                 time.Time
	RunCreated                                              time.Time
	GoVersion, GOOS, GOARCH, CPU, Runner, Image, BuildFlags string
	Procs, Samples                                          int
	Benchtime                                               string
	Fingerprints                                            map[string]string
	Metrics                                                 []string
	Complete                                                bool
}

type benchSnapshot struct {
	Cutoff   time.Time
	Manifest benchManifest
	Dir      string
	Values   map[string][]float64
}

func benchCommand(ctx context.Context, dir string, env []string, out io.Writer, command ...string) error {
	cmd := exec.CommandContext(ctx, command[0], command[1:]...) // #nosec G204 G702 -- Commands are fixed by trusted Mage callers, never artifact contents.
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout, cmd.Stderr = out, os.Stderr
	return cmd.Run()
}

func benchOutput(dir, name string, args ...string) (string, error) {
	var out bytes.Buffer
	err := benchCommand(context.Background(), dir, nil, &out, append([]string{name}, args...)...)
	return strings.TrimSpace(out.String()), err
}

// Benchmark provenance and historical fixtures already require a Git checkout.
func benchRepoRoot() (string, error) {
	return benchOutput("", "git", "rev-parse", "--show-toplevel")
}

func benchWriteJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), PrivateFile)
}

func benchSamples() (int, error) {
	value := os.Getenv("ATMOS_BENCH_SAMPLES")
	if value == "" {
		return benchMinSamples, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < benchMinSamples || n > benchMaxSamples {
		return 0, Error("ATMOS_BENCH_SAMPLES must be between 10 and 100")
	}
	return n, nil
}

// Run builds the CLI and records a complete benchmark snapshot in outputDir.
func (Bench) Run(outputDir string) error {
	root, err := benchRepoRoot()
	if err != nil {
		return err
	}
	outputDir, err = filepath.Abs(outputDir)
	if err != nil {
		return err
	}
	if err = benchPrepareOutput(outputDir); err != nil {
		return err
	}
	m, err := benchNewManifest(root)
	if err != nil {
		return err
	}
	manifestPath := filepath.Join(outputDir, "manifest.json")
	if err = benchWriteJSON(manifestPath, m); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Minute)
	defer cancel()
	if err = benchCollect(ctx, root, outputDir, m); err != nil {
		return err
	}
	return benchComplete(outputDir, m)
}

func benchComplete(outputDir string, m *benchManifest) error {
	values, err := benchReadValues(filepath.Join(outputDir, "bench.txt"))
	if err != nil {
		return err
	}
	if err = benchValidateSuite(values); err != nil {
		return err
	}
	return benchCompleteManifest(outputDir, m, values)
}

func benchCompleteManifest(outputDir string, m *benchManifest, values map[string][]float64) error {
	for key, samples := range values {
		if len(samples) != m.Samples {
			return Error("%s: expected %d samples, got %d", key, m.Samples, len(samples))
		}
		m.Metrics = append(m.Metrics, key)
	}
	sort.Strings(m.Metrics)
	m.Complete = true
	return benchWriteJSON(filepath.Join(outputDir, "manifest.json"), m)
}

func benchPrepareOutput(dir string) error {
	// A partial rerun must never retain an earlier complete manifest.
	if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err == nil {
		return Error("benchmark output already exists: %s", dir)
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.MkdirAll(dir, benchPrivateDir)
}

func benchNewManifest(root string) (*benchManifest, error) {
	m := &benchManifest{
		Schema: benchSchema, Started: time.Now().UTC(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		Repository: os.Getenv("GITHUB_REPOSITORY"), RunID: os.Getenv("GITHUB_RUN_ID"), Attempt: os.Getenv("GITHUB_RUN_ATTEMPT"),
		Event: os.Getenv("GITHUB_EVENT_NAME"), Branch: os.Getenv("GITHUB_REF_NAME"), BaseSHA: os.Getenv("ATMOS_BENCH_BASE_SHA"),
		Runner: os.Getenv("RUNNER_OS") + "/" + os.Getenv("RUNNER_ARCH"), Image: os.Getenv("ImageOS"),
		BuildFlags: "CGO_ENABLED=0 GOFLAGS= GOMAXPROCS=2", Procs: 2, Benchtime: "1s", Fingerprints: map[string]string{},
	}
	var err error
	if m.Samples, err = benchSamples(); err != nil {
		return nil, err
	}
	if m.SHA, err = benchOutput(root, "git", "rev-parse", "HEAD"); err != nil {
		return nil, err
	}
	if m.GoVersion, err = benchOutput(root, "go", "env", "GOVERSION"); err != nil {
		return nil, err
	}
	if m.CPU, err = benchCPU(root); err != nil {
		return nil, err
	}
	for _, suite := range benchSuites {
		fingerprint, err := benchFingerprint(root, suite)
		if err != nil {
			return nil, err
		}
		m.Fingerprints[benchPackage(suite.Package)] = fingerprint
	}
	return m, nil
}

func benchPackage(relative string) string {
	return "github.com/cloudposse/atmos/" + strings.TrimPrefix(relative, "./")
}

func benchCPU(root string) (string, error) {
	if runtime.GOOS != "linux" {
		return benchOutput(root, "sysctl", "-n", "machdep.cpu.brand_string")
	}
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return "", err
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.HasPrefix(line, "model name") {
			_, cpu, _ := strings.Cut(line, ":")
			return strings.TrimSpace(cpu), nil
		}
	}
	return "", Error("cannot identify benchmark CPU")
}

func benchFingerprint(root string, suite benchSuite) (string, error) {
	h := sha256.New()
	fmt.Fprintf(h, "suite-v1:%s:%v:%t", suite.Package, suite.Names, suite.CLI)
	for _, pattern := range suite.Files {
		paths, err := filepath.Glob(filepath.Join(root, pattern))
		if err != nil {
			return "", err
		}
		if len(paths) == 0 {
			return "", Error("benchmark source missing: %s", pattern)
		}
		for _, path := range paths {
			data, err := os.ReadFile(path)
			if err != nil {
				return "", err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(h, "%s\x00%s\x00", rel, data)
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func benchCollect(ctx context.Context, root, outputDir string, m *benchManifest) error {
	env := []string{"ATMOS_BENCH_OBSERVE_STORE_READS=false", "CGO_ENABLED=0", "GOFLAGS=", "GOMAXPROCS=2", "ATMOS_TELEMETRY_ENABLED=false", "ATMOS_VERSION_CHECK_ENABLED=false", "ATMOS_PRO_ENABLED=false"}
	binary := filepath.Join(outputDir, "atmos")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	if err := benchCommand(ctx, root, env, os.Stdout, "go", "build", "-o", binary, "."); err != nil {
		return err
	}
	defer os.Remove(binary)
	env = append(env, "ATMOS_BENCH_BINARY="+binary)
	f, err := os.Create(filepath.Join(outputDir, "bench.txt"))
	if err != nil {
		return err
	}
	defer f.Close()
	for _, suite := range benchSuites {
		args := []string{"test", suite.Package, "-run=^$", "-bench=^(" + strings.Join(suite.Names, "|") + ")$", "-count=" + strconv.Itoa(m.Samples), "-cpu=2", "-timeout=40m"}
		if suite.CLI {
			args = append(args, "-benchtime=1x")
		} else {
			args = append(args, "-benchtime="+m.Benchtime, "-benchmem")
		}
		if err = benchCommand(ctx, root, env, io.MultiWriter(f, os.Stdout), append([]string{"go"}, args...)...); err != nil {
			return err
		}
	}
	return f.Close()
}

func benchValidateSuite(values map[string][]float64) error {
	for _, suite := range benchSuites {
		for _, name := range suite.Names {
			prefix := benchPackage(suite.Package) + "|" + strings.TrimPrefix(name, "Benchmark")
			found := false
			for key := range values {
				if strings.HasPrefix(key, prefix+"-") || strings.HasPrefix(key, prefix+"/") {
					found = true
					break
				}
			}
			if !found {
				return Error("required benchmark missing: %s", name)
			}
		}
	}
	return benchValidateCLIMatrix(values)
}

func benchValidateCLIMatrix(values map[string][]float64) error {
	pkg := benchPackage("tests/benchmarks")
	dependents := pkg + "|CLI/DescribeAffected/Dependents/instances=1000-2|sec/op"
	if _, ok := values[dependents]; !ok {
		return Error("required CLI workload missing: %s", dependents)
	}
	// The CLI matrix must not silently lose a scale or change scenario.
	for _, size := range []int{benchSmallInstances, benchMediumInstances, benchLargeInstances} {
		for _, change := range []string{"none", "catalog", "component"} {
			key := fmt.Sprintf("%s|CLI/DescribeAffected/instances=%d/change=%s-2|sec/op", pkg, size, change)
			if _, ok := values[key]; !ok {
				return Error("required CLI workload missing: %s", key)
			}
		}
	}
	for _, name := range []string{"DescribeStacks", "ListInstances"} {
		key := pkg + "|CLI/" + name + "/instances=1000-2|sec/op"
		if _, ok := values[key]; !ok {
			return Error("required CLI workload missing: %s", key)
		}
	}
	for _, command := range []string{"DescribeAffected", "DescribeStacks"} {
		for _, unit := range []string{"sec/op", "backend-reads/op"} {
			key := pkg + "|CLI/" + command + "/StoreRepeated/calls=2200/keys=11-2|" + unit
			if _, ok := values[key]; !ok {
				return Error("required store CLI workload missing: %s", key)
			}
		}
	}
	return nil
}
