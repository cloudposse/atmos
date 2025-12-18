package ci

import (
	"context"
	"fmt"
	"os"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// GenericProviderName is the name of the generic CI provider.
	GenericProviderName = "generic"
)

// GenericProvider is a fallback CI provider for when --ci flag is used
// but no specific CI platform is detected. It writes summaries to stdout
// and outputs to environment file or stdout.
type GenericProvider struct {
	outputFile  string
	summaryFile string
}

// NewGenericProvider creates a new generic CI provider.
// It checks for ATMOS_CI_OUTPUT and ATMOS_CI_SUMMARY environment variables
// to determine where to write outputs.
func NewGenericProvider() *GenericProvider {
	defer perf.Track(nil, "ci.NewGenericProvider")()

	return &GenericProvider{
		outputFile:  os.Getenv("ATMOS_CI_OUTPUT"),
		summaryFile: os.Getenv("ATMOS_CI_SUMMARY"),
	}
}

// Name returns the provider name.
func (p *GenericProvider) Name() string {
	defer perf.Track(nil, "ci.GenericProvider.Name")()

	return GenericProviderName
}

// Detect returns false - this provider is never auto-detected.
// It's only used when CI mode is forced via --ci flag.
func (p *GenericProvider) Detect() bool {
	defer perf.Track(nil, "ci.GenericProvider.Detect")()

	return false
}

// Context returns CI metadata from environment variables.
func (p *GenericProvider) Context() (*Context, error) {
	defer perf.Track(nil, "ci.GenericProvider.Context")()

	// Try to populate context from common CI environment variables.
	ctx := &Context{
		Provider:   GenericProviderName,
		SHA:        getFirstEnv("ATMOS_CI_SHA", "GIT_COMMIT", "CI_COMMIT_SHA", "COMMIT_SHA"),
		Branch:     getFirstEnv("ATMOS_CI_BRANCH", "GIT_BRANCH", "CI_COMMIT_REF_NAME", "BRANCH_NAME"),
		Repository: getFirstEnv("ATMOS_CI_REPOSITORY", "CI_PROJECT_PATH"),
		Actor:      getFirstEnv("ATMOS_CI_ACTOR", "CI_COMMIT_AUTHOR", "USER"),
	}

	// If we have a repository, try to split into owner/name.
	if ctx.Repository != "" && strings.Contains(ctx.Repository, "/") {
		parts := strings.SplitN(ctx.Repository, "/", 2)
		if len(parts) == 2 {
			ctx.RepoOwner = parts[0]
			ctx.RepoName = parts[1]
		}
	}

	return ctx, nil
}

// GetStatus is not supported by the generic provider.
func (p *GenericProvider) GetStatus(_ context.Context, _ StatusOptions) (*Status, error) {
	defer perf.Track(nil, "ci.GenericProvider.GetStatus")()

	log.Debug("GetStatus not supported by generic CI provider")
	return nil, nil
}

// CreateCheckRun is not supported by the generic provider.
func (p *GenericProvider) CreateCheckRun(_ context.Context, _ CreateCheckRunOptions) (*CheckRun, error) {
	defer perf.Track(nil, "ci.GenericProvider.CreateCheckRun")()

	log.Debug("CreateCheckRun not supported by generic CI provider")
	return nil, nil
}

// UpdateCheckRun is not supported by the generic provider.
func (p *GenericProvider) UpdateCheckRun(_ context.Context, _ UpdateCheckRunOptions) (*CheckRun, error) {
	defer perf.Track(nil, "ci.GenericProvider.UpdateCheckRun")()

	log.Debug("UpdateCheckRun not supported by generic CI provider")
	return nil, nil
}

// OutputWriter returns an OutputWriter for the generic provider.
func (p *GenericProvider) OutputWriter() OutputWriter {
	defer perf.Track(nil, "ci.GenericProvider.OutputWriter")()

	return &GenericOutputWriter{
		outputFile:  p.outputFile,
		summaryFile: p.summaryFile,
	}
}

// GenericOutputWriter writes CI outputs for the generic provider.
type GenericOutputWriter struct {
	outputFile  string
	summaryFile string
}

// WriteOutput writes a key-value pair to CI outputs.
func (w *GenericOutputWriter) WriteOutput(key, value string) error {
	defer perf.Track(nil, "ci.GenericOutputWriter.WriteOutput")()

	if w.outputFile != "" {
		// Write to file in KEY=VALUE format (like GitHub Actions).
		f, err := os.OpenFile(w.outputFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()

		// Handle multiline values with heredoc syntax.
		if strings.Contains(value, "\n") {
			_, err = fmt.Fprintf(f, "%s<<EOF\n%s\nEOF\n", key, value)
		} else {
			_, err = fmt.Fprintf(f, "%s=%s\n", key, value)
		}
		return err
	}

	// No output file configured - log the output.
	log.Debug("CI output", "key", key, "value", value)
	return nil
}

// WriteSummary writes content to the job summary.
func (w *GenericOutputWriter) WriteSummary(content string) error {
	defer perf.Track(nil, "ci.GenericOutputWriter.WriteSummary")()

	if w.summaryFile != "" {
		// Write to file.
		f, err := os.OpenFile(w.summaryFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = f.WriteString(content)
		return err
	}

	// No summary file configured - write to stderr.
	// This makes the summary visible in local testing.
	fmt.Fprintln(os.Stderr, content)
	return nil
}

// getFirstEnv returns the value of the first environment variable that is set.
func getFirstEnv(keys ...string) string {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}
