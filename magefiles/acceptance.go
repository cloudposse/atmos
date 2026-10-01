//go:build mage

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cloudposse/atmos/internal/ci/acceptance"
	"github.com/magefile/mage/mg"
)

const repoStart = "."

var errInvalidShardCount = errors.New("TEST_SHARD_COUNT must be a positive integer")

type Acceptance mg.Namespace

// Precompile builds the acceptance test binaries once using the warm build-job cache.
func (Acceptance) Precompile(ctx context.Context, targetValue string, outputDir *string) error {
	repoRoot, target, err := targetContext(targetValue)
	if err != nil {
		return err
	}
	return acceptance.Precompile(ctx, repoRoot, target, optionalString(outputDir, "build"))
}

// Verify proves the workflow matrix and package/test assignments are complete and unique.
func (Acceptance) Verify(ctx context.Context, targetValue string, shardCount int, binaryDir *string) error {
	repoRoot, target, err := targetContext(targetValue)
	if err != nil {
		return err
	}
	return acceptance.Verify(ctx, repoRoot, target, shardCount, optionalString(binaryDir, "build"))
}

// Run executes one acceptance shard using ATMOS_TEST_SHARD and ATMOS_TEST_SHARD_COUNT.
func (Acceptance) Run(ctx context.Context, modeValue, targetValue string) error {
	repoRoot, target, err := targetContext(targetValue)
	if err != nil {
		return err
	}
	mode, err := acceptance.ParseMode(modeValue)
	if err != nil {
		return err
	}
	shardCount := firstNonEmpty(environment("ATMOS_TEST_SHARD_COUNT"), environment("TEST_SHARD_COUNT"))
	shard, err := acceptance.ParseShard(environment("ATMOS_TEST_SHARD"), shardCount)
	if err != nil {
		return err
	}
	options := acceptance.DefaultRunOptions(repoRoot, mode, target, shard)
	return acceptance.RunShard(ctx, &options)
}

type Coverage mg.Namespace

// Collect runs TEST with native subprocess coverage and merges its counters.
func (Coverage) Collect(ctx context.Context) error {
	repoRoot, err := acceptance.FindRepoRoot(repoStart)
	if err != nil {
		return err
	}
	packages, err := acceptance.SplitCommandLine(environment("TEST"))
	if err != nil {
		return err
	}
	testArgs, err := acceptance.SplitCommandLine(environment("TESTARGS"))
	if err != nil {
		return err
	}
	return acceptance.CollectCoverage(ctx, &acceptance.CoverageOptions{
		RepoRoot: repoRoot,
		Dir:      firstNonEmpty(environment("COVERAGE_DIR"), "coverage"),
		DataOut:  firstNonEmpty(environment("COVERAGE_DATA_OUT"), filepath.Join("coverage", "merged")),
		TextOut:  envDefaultAllowEmpty("COVERAGE_OUT", "coverage.out"),
		Timeout:  firstNonEmpty(environment("GO_TEST_TIMEOUT"), "40m"),
	}, packages, testArgs)
}

// Merge combines comma-separated native coverage directories.
func (Coverage) Merge(ctx context.Context, inputs string) error {
	repoRoot, err := acceptance.FindRepoRoot(repoStart)
	if err != nil {
		return err
	}
	dirs := strings.Split(inputs, ",")
	return acceptance.MergeCoverage(ctx, repoRoot,
		firstNonEmpty(environment("COVERAGE_DATA_OUT"), filepath.Join("coverage", "merged")),
		envDefaultAllowEmpty("COVERAGE_OUT", "coverage.out"), dirs)
}

// MergeShards combines every downloaded Linux shard's native coverage data.
func (Coverage) MergeShards(ctx context.Context) error {
	repoRoot, err := acceptance.FindRepoRoot(repoStart)
	if err != nil {
		return err
	}
	shardCount, err := strconv.Atoi(environment("TEST_SHARD_COUNT"))
	if err != nil || shardCount < 1 {
		return errInvalidShardCount
	}
	return acceptance.MergeCoverageShards(ctx, acceptance.CoverageShardOptions{
		RepoRoot:  repoRoot,
		Count:     shardCount,
		ShardsDir: firstNonEmpty(environment("COVERAGE_SHARDS_DIR"), filepath.Join("coverage", "shards")),
		DataOut:   firstNonEmpty(environment("COVERAGE_DATA_OUT"), filepath.Join("coverage", "merged")),
		TextOut:   envDefaultAllowEmpty("COVERAGE_OUT", "coverage.out"),
	})
}

func targetContext(value string) (string, acceptance.Target, error) {
	repoRoot, err := acceptance.FindRepoRoot(repoStart)
	if err != nil {
		return "", "", err
	}
	target, err := acceptance.ParseTarget(value)
	return repoRoot, target, err
}

func optionalString(value *string, fallback string) string {
	if value == nil || *value == "" {
		return fallback
	}
	return *value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func envDefaultAllowEmpty(name, fallback string) string {
	value, exists := os.LookupEnv(name)
	if !exists {
		return fallback
	}
	return value
}

func environment(name string) string {
	value, _ := os.LookupEnv(name)
	return value
}
