// Package generic provides a fallback CI provider for when --ci flag is used
// but no specific CI platform is detected.
package generic

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync/atomic"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/git"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	// ProviderName is the name of the generic CI provider.
	ProviderName = "generic"
)

// Ensure Provider implements provider.Provider.
var _ provider.Provider = (*Provider)(nil)

// Ensure Provider can be bound to an output writer.
var _ provider.OutputBinder = (*Provider)(nil)

func init() {
	// Self-register on package import.
	ci.Register(NewProvider())
}

// Provider is a fallback CI provider for when --ci flag is used
// but no specific CI platform is detected. It writes summaries to stdout
// and outputs to environment file or stdout.
type Provider struct {
	// writer receives local renderings; nil means the global UI channel (stderr).
	writer io.Writer
	// nextCheckRunID and nextCommentID are shared by copies made with BindOutput.
	nextCheckRunID *atomic.Int64
	nextCommentID  *atomic.Int64
	// comments remembers rendered comment markers so behavior=update can fail like a platform provider.
	// It is shared by copies made with BindOutput.
	comments *commentLedger
}

// NewProvider creates a new generic CI provider.
// ATMOS_CI_OUTPUT and ATMOS_CI_SUMMARY are read when OutputWriter is called, not here,
// so a change to the environment after construction is honored.
func NewProvider() *Provider {
	defer perf.Track(nil, "generic.NewProvider")()

	return &Provider{
		nextCheckRunID: &atomic.Int64{},
		nextCommentID:  &atomic.Int64{},
		comments:       newCommentLedger(),
	}
}

// BindOutput returns a copy of the provider whose local renderings go to w instead of
// the global UI channel. The copy shares the check-run and comment counters and the comment ledger.
func (p *Provider) BindOutput(w io.Writer) provider.Provider {
	defer perf.Track(nil, "generic.Provider.BindOutput")()

	bound := *p
	bound.writer = w
	return &bound
}

// out returns the formatter that renders to the bound writer (the UI channel when unbound).
func (p *Provider) out() *ui.Output {
	return ui.New(p.writer)
}

// render writes markdown content to out. An unbound provider (the registry instance used by
// the --ci plugin flow) emits the raw text so the output stays stable for pipelines that
// parse it; a provider bound via BindOutput renders it as terminal markdown.
func (p *Provider) render(out *ui.Output, markdown string) {
	renderMarkdown(out, p.writer != nil, markdown)
}

// renderMarkdown writes content raw, or rendered as terminal markdown when rendered is true.
func renderMarkdown(out *ui.Output, rendered bool, content string) {
	if rendered {
		out.Markdown(content)
		return
	}
	out.Writef("%s\n", content)
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
func (p *Provider) Context() (*provider.Context, error) {
	defer perf.Track(nil, "generic.Provider.Context")()

	gitRepo := git.NewDefaultGitRepo()

	// Try to populate context from common CI environment variables.
	// Fall back to git for SHA and Branch when env vars are absent.
	ctx := &provider.Context{
		Provider:   ProviderName,
		SHA:        getFirstEnvOrGit("ATMOS_CI_SHA", "GIT_COMMIT", "CI_COMMIT_SHA", "COMMIT_SHA", func() string { return gitSHA(gitRepo) }),
		Branch:     getFirstEnvOrGit("ATMOS_CI_BRANCH", "GIT_BRANCH", "CI_COMMIT_REF_NAME", "BRANCH_NAME", gitBranchFallback),
		Repository: getFirstEnv("ATMOS_CI_REPOSITORY", "CI_PROJECT_PATH"),
		Actor:      getFirstEnv("ATMOS_CI_ACTOR", "CI_COMMIT_AUTHOR", "USER"),
		// Checkout metadata from common CI conventions (GitLab CI_SERVER_URL/
		// CI_REPOSITORY_URL, Jenkins GIT_URL); empty when undeterminable.
		ServerURL: getFirstEnv("ATMOS_CI_SERVER_URL", "CI_SERVER_URL"),
		CloneURL:  getFirstEnv("ATMOS_CI_REPOSITORY_URL", "CI_REPOSITORY_URL", "GIT_URL"),
		EventName: getFirstEnv("ATMOS_CI_EVENT"),
		RunID:     getFirstEnv("ATMOS_CI_RUN_ID", "CI_JOB_ID", "BUILD_ID"),
		RunURL:    getFirstEnv("ATMOS_CI_RUN_URL", "CI_JOB_URL", "BUILD_URL"),
	}

	// ATMOS_CI_PR supplies the pull request number when the caller knows it.
	if n, err := strconv.Atoi(os.Getenv("ATMOS_CI_PR")); err == nil && n > 0 {
		ctx.PullRequest = &provider.PRInfo{
			Number:  n,
			HeadRef: ctx.Branch,
			BaseRef: os.Getenv("ATMOS_CI_BASE_REF"),
			Fork:    parseForkFlag(os.Getenv("ATMOS_CI_PR_FORK")),
		}
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
func (p *Provider) GetStatus(_ context.Context, _ provider.StatusOptions) (*provider.Status, error) {
	defer perf.Track(nil, "generic.Provider.GetStatus")()

	log.Debug("GetStatus not supported by generic CI provider")
	return nil, fmt.Errorf("%w: GetStatus is not supported by the generic CI provider", errUtils.ErrCIOperationNotSupported)
}

// OutputWriter returns an OutputWriter for the generic provider.
func (p *Provider) OutputWriter() provider.OutputWriter {
	defer perf.Track(nil, "generic.Provider.OutputWriter")()

	return &OutputWriter{
		outputFile:  os.Getenv("ATMOS_CI_OUTPUT"),
		summaryFile: os.Getenv("ATMOS_CI_SUMMARY"),
		out:         p.out(),
		rendered:    p.writer != nil,
	}
}

// OutputWriter writes CI outputs for the generic provider.
type OutputWriter struct {
	outputFile  string
	summaryFile string
	// out renders when no file is configured; nil means the global UI channel.
	out *ui.Output
	// rendered selects markdown rendering over raw text for summaries; set when the
	// provider was bound with BindOutput.
	rendered bool
}

// output returns the renderer, defaulting to the global UI channel.
func (w *OutputWriter) output() *ui.Output {
	if w.out == nil {
		return ui.New(nil)
	}
	return w.out
}

// WriteOutput writes a key-value pair to CI outputs. Without ATMOS_CI_OUTPUT it renders the line
// to the UI channel, using the heredoc form for multiline values.
func (w *OutputWriter) WriteOutput(key, value string) error {
	defer perf.Track(nil, "generic.OutputWriter.WriteOutput")()

	if w.outputFile != "" {
		// Write to file in GitHub Actions format (heredoc for multiline values).
		return provider.AppendFile(w.outputFile, provider.FormatOutputLine(key, value), errUtils.ErrCIOutputWriteFailed)
	}

	// No output file configured - render the output locally in the same form the file would hold:
	// key=value, or the heredoc form for a multiline value, so a multiline value stays unambiguous.
	w.output().Writef("%s", provider.FormatOutputLine(key, value))
	return nil
}

// WriteSummary writes content to the job summary.
func (w *OutputWriter) WriteSummary(content string) error {
	defer perf.Track(nil, "generic.OutputWriter.WriteSummary")()

	content = provider.MaskPublishedContent(content)

	if w.summaryFile != "" {
		return provider.AppendFile(w.summaryFile, content, errUtils.ErrCISummaryWriteFailed)
	}

	// No summary file configured - show the summary locally (raw unless the provider
	// was bound with BindOutput). This makes the summary visible in local testing.
	renderMarkdown(w.output(), w.rendered, content)
	return nil
}

// parseForkFlag parses ATMOS_CI_PR_FORK. An unset or invalid value means the pull request is not from a fork.
func parseForkFlag(value string) bool {
	fork, err := strconv.ParseBool(value)
	return err == nil && fork
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

// getFirstEnvOrGit returns the value of the first set env var, falling back to a git function.
func getFirstEnvOrGit(key1, key2, key3, key4 string, gitFallback func() string) string {
	if v := getFirstEnv(key1, key2, key3, key4); v != "" {
		return v
	}
	return gitFallback()
}

// gitSHA returns the current HEAD commit SHA, or empty string on failure.
func gitSHA(repo git.GitRepoInterface) string {
	sha, err := repo.GetCurrentCommitSHA()
	if err != nil {
		log.Debug("Failed to get git HEAD SHA for CI context", "error", err)
		return ""
	}
	return sha
}

// gitBranchFallback returns the current git branch name, or empty string on failure.
// This is best-effort — detached HEAD returns empty.
func gitBranchFallback() string {
	repo, err := git.GetLocalRepo()
	if err != nil {
		log.Debug("Failed to get git branch for CI context", "error", err)
		return ""
	}
	ref, err := repo.Head()
	if err != nil {
		log.Debug("Failed to get HEAD for branch resolution", "error", err)
		return ""
	}
	if !ref.Name().IsBranch() {
		return "" // Detached HEAD.
	}
	return ref.Name().Short()
}

// itoa formats n as a decimal string.
func itoa(n int) string {
	return strconv.Itoa(n)
}
