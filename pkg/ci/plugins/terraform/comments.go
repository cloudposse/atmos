package terraform

import (
	"context"
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// commentMarkerFormat is the HTML comment marker format used to find and
// update existing PR comments on repeat runs. Order: command, component,
// stack. Example: "<!-- atmos:ci:plan:vpc:plat-ue2-dev -->".
const commentMarkerFormat = "<!-- atmos:ci:%s:%s:%s -->"

// logKeyStack is the structured-log key for the stack name. Extracted as
// a constant so lint doesn't flag repeated string literals.
const logKeyStack = "stack"

// logKeyComponent is the structured-log key for the component name.
const logKeyComponent = "component"

// logCommentError logs PR comment errors. Token-related failures and
// "provider does not support comments" cases are logged at Debug level (not
// runtime failures); everything else is a warning.
func logCommentError(msg string, err error) {
	if errors.Is(err, errUtils.ErrGitHubTokenNotFound) {
		log.Debug(msg, "reason", "GITHUB_TOKEN not set")
		return
	}
	if errors.Is(err, errUtils.ErrCIOperationNotSupported) {
		log.Debug(msg, "reason", "provider does not support PR comments")
		return
	}
	log.Warn(msg, "error", err)
}

// buildCommentMarker builds the HTML comment marker used to find and update
// existing PR comments on repeat runs. The marker is unique per
// (command, stack, component) triple.
func buildCommentMarker(command, stack, component string) string {
	return fmt.Sprintf(commentMarkerFormat, command, component, stack)
}

// shouldSkipComment returns a non-empty reason when the comment post should
// be skipped — either because the event is not a PR, the repository context
// is incomplete, or no summary body is available to post.
func shouldSkipComment(ctx *plugin.HookContext, renderedSummary string) string {
	if ctx.CICtx == nil || ctx.CICtx.PullRequest == nil || ctx.CICtx.PullRequest.Number <= 0 {
		return "no PR context"
	}
	if ctx.CICtx.RepoOwner == "" || ctx.CICtx.RepoName == "" {
		return "repository owner/name missing"
	}
	if renderedSummary == "" {
		return "empty summary"
	}
	return ""
}

// postComment posts or upserts a PR comment containing the rendered plan
// summary. Reuses the summary rendered by writeSummary() so user template
// overrides in ci.templates.terraform.<command> are honored for both the
// GitHub job summary and the PR comment body. Errors are warn-only — see
// logCommentError.
func (p *Plugin) postComment(ctx *plugin.HookContext, renderedSummary string) error {
	defer perf.Track(ctx.Config, "terraform.Plugin.postComment")()

	if reason := shouldSkipComment(ctx, renderedSummary); reason != "" {
		log.Debug(
			"Skipping PR comment",
			"reason", reason,
			logKeyStack, ctx.Info.Stack,
			logKeyComponent, ctx.Info.ComponentFromArg,
		)
		return nil
	}

	marker := buildCommentMarker(ctx.Command, ctx.Info.Stack, ctx.Info.ComponentFromArg)
	opts := &provider.PostCommentOptions{
		Owner:    ctx.CICtx.RepoOwner,
		Repo:     ctx.CICtx.RepoName,
		PRNumber: ctx.CICtx.PullRequest.Number,
		Marker:   marker,
		Body:     marker + "\n" + renderedSummary,
		Behavior: resolveCommentBehavior(ctx.Config),
	}

	result, err := ctx.Provider.PostComment(context.Background(), opts)
	if err != nil {
		return err
	}

	logCommentResult(ctx.CICtx.PullRequest.Number, result)
	return nil
}

// logCommentResult logs a Debug line describing whether a comment was newly
// created or an existing one was updated.
func logCommentResult(prNumber int, result *provider.Comment) {
	action := "updated"
	url := ""
	if result != nil {
		if result.Created {
			action = "created"
		}
		url = result.URL
	}
	log.Debug("PR comment "+action, "pr", prNumber, "url", url)
}

// resolveCommentBehavior maps the config string to a provider.CommentBehavior.
// Unknown or empty values default to upsert.
func resolveCommentBehavior(cfg *schema.AtmosConfiguration) provider.CommentBehavior {
	if cfg == nil {
		return provider.CommentBehaviorUpsert
	}
	switch provider.CommentBehavior(cfg.CI.Comments.Behavior) {
	case provider.CommentBehaviorCreate:
		return provider.CommentBehaviorCreate
	case provider.CommentBehaviorUpdate:
		return provider.CommentBehaviorUpdate
	default:
		return provider.CommentBehaviorUpsert
	}
}
