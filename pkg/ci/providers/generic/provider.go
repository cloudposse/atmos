// Package generic provides a fallback CI provider for when --ci flag is used
// but no specific CI platform is detected.
package generic

import (
	"context"
	"fmt"
	"os"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// ProviderName is the name of the generic CI provider.
	ProviderName = "generic"

	// defaultFilePermissions is the file permission mode for CI output files.
	defaultFilePermissions = 0o644
)

// Ensure Provider implements ci.Provider.
var _ ci.Provider = (*Provider)(nil)

func init() {
	// Self-register on package import.
	ci.Register(NewProvider())
}

// Provider is a fallback CI provider for when --ci flag is used
// but no specific CI platform is detected. It writes summaries to stdout
// and outputs to environment file or stdout.
type Provider struct {
	outputFile  string
	summaryFile string
}

// NewProvider creates a new generic CI provider.
// It checks for ATMOS_CI_OUTPUT and ATMOS_CI_SUMMARY environment variables
// to determine where to write outputs.
func NewProvider() *Provider {
	defer perf.Track(nil, "generic.NewProvider")()

	return &Provider{
		outputFile:  os.Getenv("ATMOS_CI_OUTPUT"),
		summaryFile: os.Getenv("ATMOS_CI_SUMMARY"),
	}
}

// Name returns the provider name.
func (p *Provider) Name() string {
	defer perf.Track(nil, "generic.Provider.Name")()

	return ProviderName
}

// Detect returns false - this provider is never auto-detected.
// It's only used when CI mode is forced via --ci flag.
func (p *Provider) Detect() bool {
	defer perf.Track(nil, "generic.Provider.Detect")()

	return false
}

// Context returns CI metadata from environment variables.
func (p *Provider) Context() (*ci.Context, error) {
	defer perf.Track(nil, "generic.Provider.Context")()

	// Try to populate context from common CI environment variables.
	ctx := &ci.Context{
		Provider:   ProviderName,
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
func (p *Provider) GetStatus(_ context.Context, _ ci.StatusOptions) (*ci.Status, error) {
	defer perf.Track(nil, "generic.Provider.GetStatus")()

	log.Debug("GetStatus not supported by generic CI provider")
	return nil, fmt.Errorf("%w: GetStatus is not supported by the generic CI provider", errUtils.ErrCIOperationNotSupported)
}

// CreateCheckRun is not supported by the generic provider.
func (p *Provider) CreateCheckRun(_ context.Context, _ *ci.CreateCheckRunOptions) (*ci.CheckRun, error) {
	defer perf.Track(nil, "generic.Provider.CreateCheckRun")()

	log.Debug("CreateCheckRun not supported by generic CI provider")
	return nil, fmt.Errorf("%w: CreateCheckRun is not supported by the generic CI provider", errUtils.ErrCIOperationNotSupported)
}

// UpdateCheckRun is not supported by the generic provider.
func (p *Provider) UpdateCheckRun(_ context.Context, _ *ci.UpdateCheckRunOptions) (*ci.CheckRun, error) {
	defer perf.Track(nil, "generic.Provider.UpdateCheckRun")()

	log.Debug("UpdateCheckRun not supported by generic CI provider")
	return nil, fmt.Errorf("%w: UpdateCheckRun is not supported by the generic CI provider", errUtils.ErrCIOperationNotSupported)
}

// OutputWriter returns an OutputWriter for the generic provider.
func (p *Provider) OutputWriter() ci.OutputWriter {
	defer perf.Track(nil, "generic.Provider.OutputWriter")()

	return &OutputWriter{
		outputFile:  p.outputFile,
		summaryFile: p.summaryFile,
	}
}

// OutputWriter writes CI outputs for the generic provider.
type OutputWriter struct {
	outputFile  string
	summaryFile string
}

// WriteOutput writes a key-value pair to CI outputs.
func (w *OutputWriter) WriteOutput(key, value string) error {
	defer perf.Track(nil, "generic.OutputWriter.WriteOutput")()

	if w.outputFile != "" {
		// Write to file in KEY=VALUE format (like GitHub Actions).
		f, err := os.OpenFile(w.outputFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, defaultFilePermissions)
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
func (w *OutputWriter) WriteSummary(content string) error {
	defer perf.Track(nil, "generic.OutputWriter.WriteSummary")()

	if w.summaryFile != "" {
		// Write to file.
		f, err := os.OpenFile(w.summaryFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, defaultFilePermissions)
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
