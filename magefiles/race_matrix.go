//go:build mage

package main

import (
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"strconv"
	"strings"

	ghactions "github.com/cloudposse/atmos/pkg/github/actions"
	"github.com/cloudposse/atmos/pkg/matrix"
)

// raceShardCountEnv sets how many parallel shards RaceMatrix partitions the
// race package list into. The `race-plan` CI job (test.yml) passes
// RACE_SHARD_COUNT, the workflow-level env that also sizes the `race` job's
// own matrix -- kept as a single source of truth between the two.
const raceShardCountEnv = "RACE_SHARD_COUNT"

// raceShardSeedEnv seeds the shuffle RaceMatrix applies before round-robin
// bucketing. `go list ./...` order is stable (sorted by import path), so
// without a shuffle the same cluster of packages -- e.g. every
// pkg/toolchain subpackage, which all make real network calls -- would land
// in the same shard on every run. The CI workflow sets this to
// ${{ github.run_id }}: fixed for this run (RaceMatrix computes every
// shard's package list in one place, so there's no cross-job agreement to
// maintain) but different from the previous run, so no single shard stays
// permanently the slow one. Unset (e.g. local invocations) yields a fixed
// seed.
const raceShardSeedEnv = "ATMOS_TEST_RACE_SHARD_SEED"

var errInvalidRaceShardCount = fmt.Errorf("mage: %s must be a positive integer", raceShardCountEnv)

// raceShardEntry is one row of the GitHub Actions matrix RaceMatrix emits:
// its shard index (1-based, matching the `race` job name's "(shard N/M)"
// suffix) and its space-joined package list, fed straight into `atmos test
// race` via the TEST env override (racePackagesFromEnv in test_race.go).
type raceShardEntry struct {
	Shard    int    `json:"shard"`
	Packages string `json:"packages"`
}

// RaceMatrix emits a GitHub Actions matrix -- the same --format=matrix
// convention `atmos describe affected`/`atmos list instances` use (see
// pkg/matrix) -- partitioning the race package list across
// RACE_SHARD_COUNT shards, shuffled first (raceShardSeedEnv) so package
// order doesn't pin the same slow cluster to the same shard every run. This
// backs the `[race] plan shards` CI job (test.yml), whose output feeds the
// `race` job's `strategy.matrix.include`.
func (Test) RaceMatrix() error {
	root, err := mageRepoRoot()
	if err != nil {
		return err
	}

	packages, err := racePackages(root)
	if err != nil {
		return err
	}

	shardCount, err := strconv.Atoi(environment(raceShardCountEnv))
	if err != nil || shardCount < 1 {
		return errInvalidRaceShardCount
	}

	entries := raceShardMatrix(packages, shardCount, environment(raceShardSeedEnv))
	return matrix.WriteOutput(entries, ghactions.GetOutputPath())
}

// raceShardMatrix shuffles packages (shuffleRacePackages) and buckets them
// round-robin into shardCount groups, returning one raceShardEntry per
// shard in shard order.
func raceShardMatrix(packages []string, shardCount int, seedValue string) []raceShardEntry {
	shuffled := shuffleRacePackages(packages, seedValue)
	buckets := make([][]string, shardCount)
	for index, pkg := range shuffled {
		bucket := index % shardCount
		buckets[bucket] = append(buckets[bucket], pkg)
	}

	entries := make([]raceShardEntry, shardCount)
	for index, bucket := range buckets {
		entries[index] = raceShardEntry{
			Shard:    index + 1,
			Packages: strings.Join(bucket, " "),
		}
	}
	return entries
}

// shuffleRacePackages returns a copy of packages in a deterministic
// pseudo-random order derived from seedValue: the same seed always produces
// the same order, while different seeds produce different orders. An empty
// seedValue is its own fixed seed.
func shuffleRacePackages(packages []string, seedValue string) []string {
	shuffled := append([]string(nil), packages...)
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(seedValue))
	seed := hasher.Sum64()
	rng := rand.New(rand.NewPCG(seed, seed))
	rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
	return shuffled
}
