package vendoring

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"

	"github.com/mattn/go-isatty"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// GitDiffOptions holds options for generating a Git diff.
type GitDiffOptions struct {
	GitURI     string
	FromRef    string
	ToRef      string
	FilePath   string // Optional: filter to specific file
	Context    int    // Number of context lines
	Unified    bool   // Use unified diff format
	NoColor    bool   // Disable colorization
	OutputFile string // Optional: write to file instead of stdout
}

// buildGitDiffArgs builds the arguments for the git diff command.
func buildGitDiffArgs(opts *GitDiffOptions, colorize bool) []string {
	args := []string{"diff"}

	// Add color option
	if colorize {
		args = append(args, "--color=always")
	} else {
		args = append(args, "--color=never")
	}

	// Add context lines
	if opts.Context >= 0 {
		args = append(args, fmt.Sprintf("-U%d", opts.Context))
	}

	// Add unified format if requested (this is usually the default)
	if opts.Unified {
		args = append(args, "--unified")
	}

	// Add the refs to compare
	refRange := fmt.Sprintf("%s..%s", opts.FromRef, opts.ToRef)
	args = append(args, refRange)

	// Add file path filter if specified
	if opts.FilePath != "" {
		args = append(args, "--", opts.FilePath)
	}

	return args
}

// shouldColorizeOutput determines if output should be colorized based on:
// - no-color flag.
// - TERM environment variable.
// - TTY detection.
// - output redirection.
func shouldColorizeOutput(noColor bool, outputFile string) bool {
	// Explicit no-color flag
	if noColor {
		return false
	}

	// Writing to file
	if outputFile != "" {
		return false
	}

	// Check TERM environment variable via lookup (not BindEnv since TERM is system-level).
	term, exists := os.LookupEnv("TERM")
	if !exists || term == "dumb" || term == "" {
		return false
	}

	// Check if stdout is a TTY
	if !isatty.IsTerminal(os.Stdout.Fd()) {
		return false
	}

	return true
}

// writeOutput writes the diff output to stdout or a file.
func writeOutput(diffData []byte, outputFile string) error {
	if outputFile != "" {
		// Write to file.
		return os.WriteFile(outputFile, diffData, 0o644) //nolint:gosec,revive // Standard file permissions for generated output
	}

	// Write to stdout using data channel.
	return data.Write(string(diffData))
}

// getGitDiffBetweenRefs is a convenience function that generates a diff for a remote Git repository.
// It uses git's ability to diff remote refs without cloning.
//
//nolint:revive // Six parameters needed for Git diff configuration.
func getGitDiffBetweenRefs(atmosConfig *schema.AtmosConfiguration, gitURI string, fromRef string, toRef string, contextLines int, noColor bool) ([]byte, error) {
	defer perf.Track(atmosConfig, "exec.getGitDiffBetweenRefs")()

	// For remote diffs, we need to use a temporary shallow clone approach
	// or use git archive + diff, since git diff doesn't work with remote refs directly

	// We'll use the approach of fetching both refs and then diffing
	tempDir, err := os.MkdirTemp("", "atmos-vendor-diff-*")
	if err != nil {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrCreateTempDir, err)
	}
	defer os.RemoveAll(tempDir)

	// Initialize a bare repository
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "git", "init", "--bare", tempDir)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrGitCommandFailed, err)
	}

	// Fetch the specific refs
	cmd = exec.CommandContext(ctx, "git", "-C", tempDir, "fetch", "--depth=1", gitURI, fromRef+":"+fromRef, toRef+":"+toRef)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: failed to fetch refs: %s", errUtils.ErrGitCommandFailed, err)
	}

	// Now we can diff
	args := []string{"-C", tempDir, "diff"}

	// Add color if appropriate
	if !noColor && isatty.IsTerminal(os.Stdout.Fd()) {
		args = append(args, "--color=always")
	} else {
		args = append(args, "--color=never")
	}

	// Add context
	args = append(args, fmt.Sprintf("-U%d", contextLines))

	// Add refs
	args = append(args, fromRef, toRef)

	cmd = exec.CommandContext(ctx, "git", args...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Exit code 1 means differences found (expected)
			if exitErr.ExitCode() == 1 && len(output) > 0 {
				return output, nil
			}
			return nil, fmt.Errorf("%w: %s", errUtils.ErrGitDiffFailed, string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("%w: %s", errUtils.ErrGitDiffFailed, err)
	}

	return output, nil
}

// ansiRegex matches ANSI escape codes (SGR sequences).
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// stripANSICodes removes ANSI escape codes from byte data.
func stripANSICodes(data []byte) []byte {
	return ansiRegex.ReplaceAll(data, nil)
}
