package bench

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
)

const (
	benchFieldMode         = "field-test"
	benchFieldMeasurements = "measurements"
)

// FieldTest rebuilds comma-separated historical refs and measures each binary
// with this checkout's identical CLI harness. The first ref is the known-good
// baseline. This is an explicit local experiment, never a CI baseline source.
func (Bench) FieldTest(refs, outputDir string) error {
	root, err := benchRepoRoot()
	if err != nil {
		return err
	}
	outputDir, err = filepath.Abs(outputDir)
	if err != nil {
		return err
	}
	if err = benchFieldOutput(outputDir); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Hour)
	defer cancel()
	versions, err := benchFieldVersions(root, refs)
	if err != nil {
		return err
	}
	// Finish every build before collecting timings; never compile concurrently
	// with another version's measurements.
	for _, version := range versions {
		if err = benchFieldBuild(ctx, root, outputDir, version); err != nil {
			return err
		}
	}
	for _, version := range versions {
		if err = benchFieldMeasure(ctx, root, outputDir, version); err != nil {
			return err
		}
	}
	return benchFieldReports(outputDir, versions)
}

type benchFieldVersion struct{ Ref, SHA string }

func benchFieldVersions(root, refs string) ([]benchFieldVersion, error) {
	var versions []benchFieldVersion
	seen := map[string]bool{}
	for _, ref := range strings.Split(refs, ",") {
		ref = strings.TrimSpace(ref)
		if ref == "" || strings.HasPrefix(ref, "-") {
			return nil, Error("invalid field-test ref")
		}
		sha, err := benchOutput(root, "git", "rev-parse", "--verify", ref+"^{commit}")
		if err != nil {
			return nil, err
		}
		if !githubapi.SHAPattern.MatchString(sha) || seen[sha] {
			return nil, Error("invalid or duplicate field-test revision: %s", ref)
		}
		seen[sha] = true
		versions = append(versions, benchFieldVersion{Ref: ref, SHA: sha})
	}
	if len(versions) < 2 {
		return nil, Error("field test needs a baseline and at least one candidate ref")
	}
	return versions, nil
}

func benchFieldOutput(dir string) error {
	entries, err := os.ReadDir(dir)
	if err == nil && len(entries) > 0 {
		return Error("field-test output must be empty")
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.MkdirAll(filepath.Join(dir, "binaries"), benchPrivateDir)
}

func benchFieldBuild(ctx context.Context, root, dir string, version benchFieldVersion) error {
	source := filepath.Join(dir, "source-"+version.SHA)
	if err := benchCommand(ctx, root, nil, os.Stdout, "git", "worktree", "add", "--detach", source, version.SHA); err != nil {
		return err
	}
	defer func() {
		if err := benchCommand(context.Background(), root, nil, os.Stderr, "git", "worktree", "remove", "--force", source); err != nil {
			fmt.Fprintln(os.Stderr, "field-test worktree cleanup:", err)
		}
	}()
	log, err := os.Create(filepath.Join(dir, "build-"+version.SHA+".log"))
	if err != nil {
		return err
	}
	defer log.Close()
	fmt.Printf("Building field-test revision %s (%s)\n", version.Ref, version.SHA)
	return benchCommand(ctx, source, []string{"CGO_ENABLED=0", "GOFLAGS=", "GOMAXPROCS=2", "GOTOOLCHAIN=local"}, log, "go", "build", "-o", filepath.Join(dir, "binaries", version.SHA), ".")
}

func benchFieldMeasure(ctx context.Context, root, dir string, version benchFieldVersion) error {
	target := filepath.Join(dir, benchFieldMeasurements, version.SHA)
	if err := os.MkdirAll(target, benchPrivateDir); err != nil {
		return err
	}
	m, err := benchNewManifest(root)
	if err != nil {
		return err
	}
	m.SHA, m.Mode, m.SourceRef = version.SHA, benchFieldMode, version.Ref
	m.HarnessSHA, err = benchOutput(root, "git", "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	m.Event, m.RunID, m.Attempt, m.Branch = "", "", "", ""
	m.BuildFlags += " GOTOOLCHAIN=local; observe-store-reads"
	m.Benchtime = "1x"
	if err = benchWriteJSON(filepath.Join(target, "manifest.json"), m); err != nil {
		return err
	}
	if err = benchFieldCollect(ctx, root, dir, version, m); err != nil {
		return err
	}
	values, err := benchReadValues(filepath.Join(target, "bench.txt"))
	if err != nil {
		return err
	}
	return benchCompleteManifest(target, m, values)
}

func benchFieldCollect(ctx context.Context, root, dir string, version benchFieldVersion, m *benchManifest) error {
	target := filepath.Join(dir, benchFieldMeasurements, version.SHA)
	out, err := os.Create(filepath.Join(target, "bench.txt"))
	if err != nil {
		return err
	}
	defer out.Close()
	filter := os.Getenv("ATMOS_BENCH_FIELD_FILTER")
	if filter == "" {
		filter = "^BenchmarkCLI$/^(DescribeAffected|DescribeStacks)$/^StoreRepeated$"
	}
	env := []string{"CGO_ENABLED=0", "GOFLAGS=", "GOMAXPROCS=2", "GOTOOLCHAIN=local", "ATMOS_BENCH_OBSERVE_STORE_READS=true", "ATMOS_BENCH_BINARY=" + filepath.Join(dir, "binaries", version.SHA)}
	fmt.Printf("Measuring field-test revision %s (%s)\n", version.Ref, version.SHA)
	return benchCommand(ctx, root, env, io.MultiWriter(out, os.Stdout), "go", "test", "./tests/benchmarks", "-run=^$", "-bench="+filter, "-count="+fmt.Sprint(m.Samples), "-benchtime=1x", "-cpu=2", "-timeout=60m")
}

func benchFieldReports(dir string, versions []benchFieldVersion) error {
	history := filepath.Join(dir, "history")
	base := filepath.Join(dir, benchFieldMeasurements, versions[0].SHA)
	if err := os.MkdirAll(filepath.Join(history, versions[0].SHA), benchPrivateDir); err != nil {
		return err
	}
	for _, name := range []string{"manifest.json", "bench.txt"} {
		data, err := os.ReadFile(filepath.Join(base, name))
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(history, versions[0].SHA, name), data, PrivateFile); err != nil { // #nosec G703 -- Caller-selected output directory, validated SHA, and fixed data filenames.
			return err
		}
	}
	var summary strings.Builder
	fmt.Fprintf(&summary, "# Historical benchmark field test\n\nKnown-good reference: `%s` (`%s`). Every candidate uses the same current CLI harness and local Go toolchain. These are rebuilt local results, not stored CI baselines.\n\n", versions[0].Ref, versions[0].SHA)
	for _, version := range versions[1:] {
		reportDir := filepath.Join(dir, "comparisons", version.SHA)
		if err := (Bench{}).Compare(history, filepath.Join(dir, benchFieldMeasurements, version.SHA), reportDir); err != nil {
			return err
		}
		data, err := os.ReadFile(filepath.Join(reportDir, "report.md"))
		if err != nil {
			return err
		}
		fmt.Fprintf(&summary, "## %s\n\n%s\n", Escape(version.Ref), data)
	}
	return os.WriteFile(filepath.Join(dir, "report.md"), []byte(summary.String()), PrivateFile)
}
