package acceptance

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Mode selects ordinary test execution or native coverage collection.
type Mode string

const (
	ModeTest     Mode = "test"
	ModeCoverage Mode = "coverage"
)

func ParseMode(value string) (Mode, error) {
	mode := Mode(value)
	switch mode {
	case ModeTest, ModeCoverage:
		return mode, nil
	default:
		return "", fmt.Errorf("%w: mode %q", errInvalidConfiguration, value)
	}
}

type RunOptions struct {
	RepoRoot       string
	Mode           Mode
	Target         Target
	Shard          Shard
	BuildDir       string
	CoverageRoot   string
	GoTestTimeout  string
	CmdTestBinary  string
	TestsBinary    string
	ExecTestBinary string
}

type sourceTestGroup struct {
	name     string
	packages []string
	testArgs []string
}

//nolint:cyclop,gocognit,nestif,revive // Ordered orchestration intentionally keeps the shard phases visible in one place.
func RunShard(ctx context.Context, options *RunOptions) error {
	runner := newCommandRunner()
	allPackages, err := listPackages(ctx, runner, options.RepoRoot)
	if err != nil {
		return err
	}
	packages := shardPackages(allPackages, options.Target, options.Shard)
	coverageInputs := make([]string, 0, 3)
	if options.Target == TargetWindows && options.Mode == ModeTest {
		if err := runWindowsTests(ctx, runner, options); err != nil {
			return err
		}
	} else {
		input, runErr := runSourceTestGroup(ctx, runner, options, sourceTestGroup{
			name: "tests", packages: []string{"./tests"}, testArgs: sourceTestArgs(options.Shard),
		})
		if runErr != nil {
			return runErr
		}
		if input != "" {
			coverageInputs = append(coverageInputs, input)
		}
	}

	if len(packages) > 0 {
		input, runErr := runSourceTestGroup(ctx, runner, options, sourceTestGroup{name: "pkgs", packages: packages})
		if runErr != nil {
			return runErr
		}
		if input != "" {
			coverageInputs = append(coverageInputs, input)
		}
	}

	if options.Target == TargetWindows && options.Mode == ModeTest {
		if err := runWindowsExecTests(ctx, runner, options); err != nil {
			return err
		}
	}
	if options.Shard.Index == 1 {
		input, runErr := runCmdTests(ctx, runner, options)
		if runErr != nil {
			return runErr
		}
		if input != "" {
			coverageInputs = append(coverageInputs, input)
		}
	}

	if options.Mode == ModeCoverage {
		return MergeCoverage(ctx, options.RepoRoot,
			filepath.Join(options.CoverageRoot, fmt.Sprintf("shard-%d", options.Shard.Index)), "", coverageInputs)
	}
	return nil
}

func runSourceTestGroup(
	ctx context.Context,
	runner commandRunner,
	options *RunOptions,
	group sourceTestGroup,
) (string, error) {
	if options.Mode == ModeCoverage {
		dataDir := filepath.Join(options.CoverageRoot, fmt.Sprintf("shard-%d-%s", options.Shard.Index, group.name))
		err := CollectCoverage(ctx, &CoverageOptions{
			RepoRoot: options.RepoRoot,
			Dir:      filepath.Join(options.CoverageRoot, fmt.Sprintf("shard-%d-%s-work", options.Shard.Index, group.name)),
			DataOut:  dataDir,
			TextOut:  "",
			Timeout:  options.GoTestTimeout,
		}, group.packages, group.testArgs)
		return dataDir, err
	}
	args := []string{"test"}
	args = append(args, group.packages...)
	args = append(args, group.testArgs...)
	args = append(args, "-timeout", defaultValue(options.GoTestTimeout, defaultTestTimeout))
	return "", runner.run(ctx, options.RepoRoot, goCommandEnvironment(), "go", args...)
}

func runWindowsTests(ctx context.Context, runner commandRunner, options *RunOptions) error {
	binary := absoluteFromRoot(options.RepoRoot, defaultValue(options.TestsBinary, filepath.Join(options.BuildDir, "tests.test.exe")))
	if err := requireFile(binary); err != nil {
		return err
	}
	dir := filepath.Join(options.RepoRoot, "tests")
	tests, err := listRunnableTests(ctx, runner, dir, binary)
	if err != nil {
		return fmt.Errorf("list tests package cases: %w", err)
	}
	assigned := windowsTestsForShard(tests, options.Shard)
	args := []string{"-test.run=" + testRunPattern(assigned), "-test.timeout=" + defaultValue(options.GoTestTimeout, defaultTestTimeout)}
	return runner.run(ctx, dir, nil, binary, args...)
}

func runWindowsExecTests(ctx context.Context, runner commandRunner, options *RunOptions) error {
	binary := absoluteFromRoot(options.RepoRoot, defaultValue(options.ExecTestBinary, filepath.Join(options.BuildDir, "internal-exec.test.exe")))
	if err := requireFile(binary); err != nil {
		return err
	}
	dir := filepath.Join(options.RepoRoot, "internal", "exec")
	tests, err := listRunnableTests(ctx, runner, dir, binary)
	if err != nil {
		return fmt.Errorf("list internal/exec cases: %w", err)
	}
	assigned := assignedTests(tests, options.Shard, nil)
	if len(assigned) == 0 {
		return nil
	}
	args := []string{"-test.run=" + testRunPattern(assigned), "-test.timeout=" + defaultValue(options.GoTestTimeout, defaultTestTimeout)}
	return runner.run(ctx, dir, nil, binary, args...)
}

func runCmdTests(ctx context.Context, runner commandRunner, options *RunOptions) (string, error) {
	extension := ""
	if options.Target == TargetWindows {
		extension = ".exe"
	}
	binary := absoluteFromRoot(options.RepoRoot, defaultValue(options.CmdTestBinary, filepath.Join(options.BuildDir, "cmd.test"+extension)))
	if err := requireFile(binary); err != nil {
		return "", err
	}
	if options.Target != TargetWindows {
		if err := os.Chmod(binary, directoryPermissions); err != nil {
			return "", fmt.Errorf("make cmd test binary executable: %w", err)
		}
	}
	dir := filepath.Join(options.RepoRoot, "cmd")
	args := []string{"-test.v", "-test.timeout=10m"}
	coverDir := ""
	if options.Mode == ModeCoverage {
		coverDir = filepath.Join(options.CoverageRoot, fmt.Sprintf("shard-%d-cmd", options.Shard.Index))
		if err := os.MkdirAll(coverDir, directoryPermissions); err != nil {
			return "", fmt.Errorf("create cmd coverage directory: %w", err)
		}
		args = append(args, "-test.gocoverdir="+absoluteFromRoot(options.RepoRoot, coverDir))
	} else {
		temporary, err := os.MkdirTemp("", "atmos-cmd-coverage-")
		if err != nil {
			return "", fmt.Errorf("create cmd coverage directory: %w", err)
		}
		defer func() { _ = os.RemoveAll(temporary) }()
		coverDir = temporary
	}
	absCoverDir := absoluteFromRoot(options.RepoRoot, coverDir)
	if err := runner.run(ctx, dir, []string{"GOCOVERDIR=" + absCoverDir}, binary, args...); err != nil {
		return "", err
	}
	if options.Mode == ModeCoverage {
		return coverDir, nil
	}
	return "", nil
}

func Precompile(ctx context.Context, repoRoot string, target Target, outputDir string) error {
	runner := newCommandRunner()
	outputDir = absoluteFromRoot(repoRoot, defaultValue(outputDir, "build"))
	if err := os.MkdirAll(outputDir, directoryPermissions); err != nil {
		return fmt.Errorf("create build directory: %w", err)
	}
	extension := ""
	if target == TargetWindows {
		extension = ".exe"
	}
	if err := runner.run(ctx, repoRoot, goCommandEnvironment(), "go", "test", "-c", "-covermode=atomic",
		"-o", filepath.Join(outputDir, "cmd.test"+extension), "./cmd"); err != nil {
		return err
	}
	if target != TargetWindows {
		return nil
	}
	for _, testBinary := range []struct {
		name    string
		pkgPath string
	}{
		{name: "tests.test.exe", pkgPath: "./tests"},
		{name: "internal-exec.test.exe", pkgPath: "./internal/exec"},
	} {
		if err := runner.run(ctx, repoRoot, goCommandEnvironment(), "go", "test", "-c",
			"-o", filepath.Join(outputDir, testBinary.name), testBinary.pkgPath); err != nil {
			return err
		}
	}
	return nil
}

func absoluteFromRoot(repoRoot, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(repoRoot, path)
}

func requireFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("required file %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%w: %s is a directory", errRequiredArtifact, path)
	}
	return nil
}

func DefaultRunOptions(repoRoot string, mode Mode, target Target, shard Shard) RunOptions {
	return RunOptions{
		RepoRoot:       repoRoot,
		Mode:           mode,
		Target:         target,
		Shard:          shard,
		BuildDir:       defaultValue(environment("BUILD_DIR"), "."),
		CoverageRoot:   defaultValue(environment("COVERAGE_ROOT"), "coverage"),
		GoTestTimeout:  defaultValue(environment("GO_TEST_TIMEOUT"), defaultTestTimeout),
		CmdTestBinary:  environment("CMD_TEST_BIN"),
		TestsBinary:    environment("TESTS_TEST_BIN"),
		ExecTestBinary: environment("INTERNAL_EXEC_TEST_BIN"),
	}
}

func SplitCommandLine(value string) []string {
	return strings.Fields(strings.ReplaceAll(value, "\n", " "))
}
