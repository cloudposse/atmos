//go:build mage

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/magefile/mage/sh"

	"github.com/cloudposse/atmos/internal/ci/acceptance"
	"github.com/cloudposse/atmos/internal/ci/testshard"
)

const (
	raceToolchainPackage = "github.com/cloudposse/atmos/pkg/toolchain"
	raceReportDirMode    = 0o755
	raceReportFileMode   = 0o600
)

// runRaceShard keeps test discovery on the workers so the lightweight planner
// never adds a race compilation barrier before the whole suite can start.
func runRaceShard(root string) error {
	shard, err := acceptance.ParseShard(os.Getenv("ATMOS_TEST_RACE_SHARD"), os.Getenv(raceShardCountEnv))
	if err != nil {
		return err
	}
	args, err := raceShardArgs()
	if err != nil {
		return err
	}
	packages, err := acceptance.SplitCommandLine(os.Getenv("TEST"))
	if err != nil {
		return err
	}
	for _, pkg := range packages {
		if pkg == raceToolchainPackage {
			return fmt.Errorf("%w: toolchain also appears in ordinary packages", testshard.ErrPlan)
		}
	}
	// Continue with ordinary packages even if discovery or the toolchain group fails.
	toolchainErr := runToolchainShard(root, shard, args)
	var generalErr error
	if len(packages) > 0 {
		generalArgs := append(append([]string{"test"}, args...), packages...)
		generalErr = runRaceCommand(root, "packages", generalArgs)
	}
	return errors.Join(toolchainErr, generalErr)
}

func raceShardArgs() ([]string, error) {
	extra, err := acceptance.SplitCommandLine(os.Getenv("TESTARGS"))
	if err != nil {
		return nil, err
	}
	// Inventory-changing flags would make the completeness guarantee misleading.
	for _, arg := range extra {
		flag := strings.SplitN(strings.TrimLeft(arg, "-"), "=", 2)[0]
		switch flag {
		case "v", "json":
		default:
			return nil, fmt.Errorf("%w: sharded race TESTARGS only supports -v and -json, got %q", testshard.ErrPlan, arg)
		}
	}
	parallel := os.Getenv(raceParallelEnv)
	if parallel == "" {
		parallel = raceParallelDefault
	}
	return []string{"-race", "-json", "-count=1", "-shuffle=on", "-parallel=" + parallel, "-timeout=" + raceTestTimeout}, nil
}

func runToolchainShard(root string, shard acceptance.Shard, args []string) error {
	started := time.Now()
	output, err := sh.Output("go", "-C", root, "test", "-race", "-list", ".", raceToolchainPackage)
	if err != nil {
		return fmt.Errorf("list toolchain race tests: %w", err)
	}
	names, err := testshard.Discover(output)
	if err != nil {
		return err
	}
	weights, err := readRaceWeights(root)
	if err != nil {
		return err
	}
	groups, err := testshard.Plan(names, shard.Count, weights)
	if err != nil {
		return err
	}
	selected := groups[shard.Index-1]
	if err := writeRacePlan(shard, names, selected, time.Since(started)); err != nil {
		return err
	}
	if len(selected) == 0 {
		return nil
	}
	testArgs := append([]string{"test"}, args...)
	testArgs = append(testArgs, "-run", testshard.Pattern(selected), raceToolchainPackage)
	return runRaceCommand(root, "toolchain", testArgs)
}

func readRaceWeights(root string) (map[string]float64, error) {
	data, err := os.ReadFile(filepath.Join(root, ".github", "test-timings", "toolchain.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read race timing hints: %w", err)
	}
	var manifest struct {
		Seconds map[string]float64 `json:"seconds"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse race timing hints: %w", err)
	}
	return manifest.Seconds, nil
}

func writeRacePlan(shard acceptance.Shard, all, selected []string, discovery time.Duration) error {
	directory := os.Getenv("ATMOS_TEST_RACE_REPORT_DIR")
	if directory == "" {
		return nil
	}
	if err := os.MkdirAll(directory, raceReportDirMode); err != nil { // #nosec G703 -- report directory is explicitly selected by the caller/CI, not test output.
		return err
	}
	plan := struct {
		Shard            int      `json:"shard"`
		Count            int      `json:"count"`
		Discovered       []string `json:"discovered"`
		Selected         []string `json:"selected"`
		DiscoverySeconds float64  `json:"discovery_seconds"`
	}{shard.Index, shard.Count, all, selected, discovery.Seconds()}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, "plan-"+strconv.Itoa(shard.Index)+".json"), append(data, '\n'), raceReportFileMode) // #nosec G703 -- fixed filename under the caller-selected report directory.
}

// runRaceCommand streams JSON events to the log and an artifact, including on
// failure, then records a compact summary without double-counting subtests.
func runRaceCommand(root, name string, args []string) error {
	directory := os.Getenv("ATMOS_TEST_RACE_REPORT_DIR")
	if directory == "" {
		return runIn(root, nil, "go", args...)
	}
	if err := os.MkdirAll(directory, raceReportDirMode); err != nil { // #nosec G703 -- report directory is explicitly selected by the caller/CI, not test output.
		return err
	}
	path := filepath.Join(directory, name+".jsonl")
	output, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, raceReportFileMode) // #nosec G703 -- fixed filename under the caller-selected report directory.
	if err != nil {
		return err
	}
	cmd := exec.Command("go", args...) // #nosec G204 G702 -- fixed go executable, argument vector from race runner; no shell.
	cmd.Dir = root
	cmd.Stdout = io.MultiWriter(os.Stdout, output)
	cmd.Stderr = os.Stderr
	started := time.Now()
	runErr := cmd.Run()
	closeErr := output.Close()
	summaryErr := summarizeRace(path, time.Since(started))
	if runErr != nil {
		runErr = fmt.Errorf("race %s: %w", name, runErr)
	}
	return errors.Join(runErr, closeErr, summaryErr)
}

func summarizeRace(path string, elapsed time.Duration) error {
	input, err := os.Open(path) // #nosec G703 -- reads the JSON event file this runner just created.
	if err != nil {
		return err
	}
	defer input.Close()
	timings, err := testshard.ReadTimings(input)
	if err != nil {
		return err
	}
	summary := struct {
		testshard.Timings
		WallSeconds float64 `json:"wall_seconds"`
	}{timings, elapsed.Seconds()}
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(strings.TrimSuffix(path, ".jsonl")+".json", append(data, '\n'), raceReportFileMode) // #nosec G703 -- fixed filename under the caller-selected report directory.
}
