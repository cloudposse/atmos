package acceptance

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// CoverageOptions configures native Go coverage collection.
type CoverageOptions struct {
	RepoRoot string
	Dir      string
	DataOut  string
	TextOut  string
	Timeout  string
}

// CoverageShardOptions configures merging downloaded native coverage artifacts.
type CoverageShardOptions struct {
	RepoRoot  string
	Count     int
	ShardsDir string
	DataOut   string
	TextOut   string
}

func CollectCoverage(ctx context.Context, options *CoverageOptions, packages, testArgs []string) error {
	runner := newCommandRunner()
	if len(packages) == 0 {
		all, err := listPackages(ctx, runner, options.RepoRoot)
		if err != nil {
			return err
		}
		packages = all
	}
	integrationDir := filepath.Join(options.Dir, "integration")
	unitDir := filepath.Join(options.Dir, "unit")
	absIntegration, absUnit, err := prepareCoverageDirectories(options.Dir)
	if err != nil {
		return err
	}

	if err := writeStatus("Running tests with subprocess coverage collection\n"); err != nil {
		return err
	}
	args := []string{"test"}
	args = append(args, packages...)
	args = append(args, "-cover", "-covermode=atomic", "-coverpkg=./...")
	args = append(args, testArgs...)
	args = append(args, "-timeout", defaultValue(options.Timeout, defaultTestTimeout), "-args", "-test.gocoverdir="+absUnit)
	if err := runner.run(ctx, options.RepoRoot, goCommandEnvironment("GOCOVERDIR="+absIntegration), "go", args...); err != nil {
		return err
	}

	inputs := []string{unitDir}
	if hasCoverageMetadata(integrationDir) {
		inputs = append(inputs, integrationDir)
	}
	return MergeCoverage(ctx, options.RepoRoot, options.DataOut, options.TextOut, inputs)
}

func MergeCoverage(ctx context.Context, repoRoot, dataOut, textOut string, inputs []string) error {
	if err := validateCoverageInputs(inputs); err != nil {
		return err
	}
	if err := resetDirectory(dataOut); err != nil {
		return err
	}
	runner := newCommandRunner()
	if err := runner.run(ctx, repoRoot, nil, "go", "tool", "covdata", "merge", "-pcombine", "-i="+strings.Join(inputs, ","), "-o="+dataOut); err != nil {
		return err
	}
	if textOut != "" {
		if err := writeCoverageText(ctx, runner, repoRoot, dataOut, textOut); err != nil {
			return err
		}
	}
	return writeStatus("Native coverage data generated: %s\n", dataOut)
}

func MergeCoverageShards(ctx context.Context, options CoverageShardOptions) error {
	inputs := make([]string, 0, options.Count)
	for shard := 1; shard <= options.Count; shard++ {
		shardDir := filepath.Join(options.ShardsDir, "shard-"+strconv.Itoa(shard))
		metadata, err := findCoverageMetadata(shardDir)
		if err != nil {
			return fmt.Errorf("find coverage for shard %d: %w", shard, err)
		}
		if metadata == "" {
			return fmt.Errorf("%w: no native metadata found for shard %d", errCoverageData, shard)
		}
		inputs = append(inputs, filepath.Dir(metadata))
	}
	return MergeCoverage(ctx, options.RepoRoot, options.DataOut, options.TextOut, inputs)
}

func prepareCoverageDirectories(root string) (string, string, error) {
	if err := resetDirectory(root); err != nil {
		return "", "", err
	}
	integrationDir := filepath.Join(root, "integration")
	unitDir := filepath.Join(root, "unit")
	for _, dir := range []string{integrationDir, unitDir} {
		if err := os.MkdirAll(dir, directoryPermissions); err != nil {
			return "", "", fmt.Errorf("create coverage directory: %w", err)
		}
	}
	absIntegration, err := filepath.Abs(integrationDir)
	if err != nil {
		return "", "", fmt.Errorf("resolve integration coverage directory: %w", err)
	}
	absUnit, err := filepath.Abs(unitDir)
	if err != nil {
		return "", "", fmt.Errorf("resolve unit coverage directory: %w", err)
	}
	return absIntegration, absUnit, nil
}

func validateCoverageInputs(inputs []string) error {
	if len(inputs) == 0 {
		return fmt.Errorf("%w: at least one data directory is required", errCoverageData)
	}
	for _, input := range inputs {
		if !hasCoverageMetadata(input) {
			return fmt.Errorf("%w: no native metadata found in %s", errCoverageData, input)
		}
	}
	return nil
}

func writeCoverageText(ctx context.Context, runner commandRunner, repoRoot, dataOut, textOut string) error {
	raw, err := os.CreateTemp("", "atmos-coverage-*.out")
	if err != nil {
		return fmt.Errorf("create temporary coverage profile: %w", err)
	}
	rawPath := raw.Name()
	if closeErr := raw.Close(); closeErr != nil {
		return fmt.Errorf("close temporary coverage profile: %w", closeErr)
	}
	defer func() { _ = os.Remove(rawPath) }()
	if err := runner.run(ctx, repoRoot, nil, "go", "tool", "covdata", "textfmt", "-i="+dataOut, "-o="+rawPath); err != nil {
		return err
	}
	if err := filterCoverageProfile(rawPath, textOut); err != nil {
		return err
	}
	return writeStatus("Coverage report generated: %s\n", textOut)
}

func resetDirectory(path string) error {
	clean := filepath.Clean(path)
	if clean == "." || clean == string(filepath.Separator) || filepath.VolumeName(clean)+string(filepath.Separator) == clean {
		return fmt.Errorf("%w: refusing unsafe output directory %q", errInvalidConfiguration, path)
	}
	if err := os.RemoveAll(clean); err != nil {
		return fmt.Errorf("remove output directory %s: %w", clean, err)
	}
	if err := os.MkdirAll(clean, directoryPermissions); err != nil {
		return fmt.Errorf("create output directory %s: %w", clean, err)
	}
	return nil
}

func hasCoverageMetadata(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "covmeta.") {
			return true
		}
	}
	return false
}

func findCoverageMetadata(root string) (string, error) {
	var result string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "covmeta.") {
			result = path
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return result, nil
}

func filterCoverageProfile(source, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), directoryPermissions); err != nil {
		return fmt.Errorf("create coverage profile directory: %w", err)
	}
	in, err := os.Open(source) //nolint:gosec // Source is a Go-generated profile in a caller-controlled temporary directory.
	if err != nil {
		return fmt.Errorf("open raw coverage profile: %w", err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("create coverage profile: %w", err)
	}
	defer func() { _ = out.Close() }()
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "mock_") {
			continue
		}
		if _, err := io.WriteString(out, scanner.Text()+"\n"); err != nil { //nolint:gosec // This copies Go coverage text; it is not rendered as HTML.
			return fmt.Errorf("write coverage profile: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read raw coverage profile: %w", err)
	}
	return nil
}

func defaultValue(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
