//go:build mage

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/magefile/mage/mg"

	"github.com/cloudposse/atmos/internal/ci/rerun"
)

// CI groups targets that CI workflows dispatch to for decisions that are
// easier to keep in typed, unit-tested Go than in workflow YAML.
type CI mg.Namespace

// stuckGapEnv optionally overrides rerun.DefaultStuckGap as a Go duration
// (for example "300s"); .github/actions/classify-infra-failures sets it from
// its stuck-gap-seconds input.
const stuckGapEnv = "ATMOS_CI_STUCK_GAP"

// githubOutputDelimiter closes the multi-line `table` value written to
// GITHUB_OUTPUT; job names never contain it.
const githubOutputDelimiter = "ATMOS_CLASSIFY_EOF"

// githubOutputFileMode is used when GITHUB_OUTPUT does not exist yet (only
// outside the Actions runner, e.g. in tests).
const githubOutputFileMode = 0o644

var errInvalidStuckGap = errors.New("mage: " + stuckGapEnv + " must be a positive Go duration such as 300s")

// ClassifyInfraFailures classifies every non-success job in jobsFile (the
// output of `gh api repos/{owner}/{repo}/actions/runs/{id}/attempts/{n}/jobs
// --paginate`) as an infrastructure zombie or a real failure, prints one TSV
// line (`job<TAB>conclusion<TAB>class`) per job to stdout and the verdict to
// stderr. When GITHUB_OUTPUT is set it also records `verdict`
// (rerun|no-rerun|no-jobs), `reason` and the multi-line `table` for the
// calling workflow step. See internal/ci/rerun for the rules.
func (CI) ClassifyInfraFailures(jobsFile string) error {
	return classifyInfraFailures(jobsFile, os.Stdout, os.Stderr, os.Getenv("GITHUB_OUTPUT"), os.Getenv(stuckGapEnv))
}

func classifyInfraFailures(jobsFile string, stdout, stderr io.Writer, outputPath, stuckGap string) error {
	opts, err := stuckGapOptions(stuckGap)
	if err != nil {
		return err
	}
	f, err := os.Open(jobsFile)
	if err != nil {
		return fmt.Errorf("mage: open jobs file: %w", err)
	}
	defer f.Close()
	jobs, err := rerun.DecodeJobs(f)
	if err != nil {
		return err
	}

	classified := rerun.Classify(jobs, opts)
	outcome, reason := rerun.Verdict(classified)
	var table bytes.Buffer
	if err := rerun.WriteTSV(&table, classified); err != nil {
		return err
	}
	if _, err := stdout.Write(table.Bytes()); err != nil {
		return fmt.Errorf("mage: write table: %w", err)
	}
	fmt.Fprintf(stderr, "verdict: %s (%s)\n", outcome, reason) //nolint:gosec // Plain-text log line; nothing here is rendered as HTML.

	if outputPath == "" {
		return nil
	}
	return appendGitHubOutput(outputPath, outcome, reason, table.String())
}

func stuckGapOptions(value string) (rerun.Options, error) {
	if value == "" {
		return rerun.Options{}, nil
	}
	gap, err := time.ParseDuration(value)
	if err != nil || gap <= 0 {
		return rerun.Options{}, fmt.Errorf("%w: got %q", errInvalidStuckGap, value)
	}
	return rerun.Options{StuckGap: gap}, nil
}

func appendGitHubOutput(path string, outcome rerun.Outcome, reason, table string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, githubOutputFileMode) //nolint:gosec // Path is GITHUB_OUTPUT, set by the Actions runner, not user input.
	if err != nil {
		return fmt.Errorf("mage: open GITHUB_OUTPUT: %w", err)
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "verdict=%s\nreason=%s\ntable<<%s\n%s%s\n", outcome, reason, githubOutputDelimiter, table, githubOutputDelimiter) //nolint:gosec // GITHUB_OUTPUT is a plain-text key=value file, not HTML.
	if err != nil {
		return fmt.Errorf("mage: write GITHUB_OUTPUT: %w", err)
	}
	return nil
}
