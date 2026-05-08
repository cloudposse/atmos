package github

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/git"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// DefaultRef is the fallback git reference when no base can be resolved.
	defaultRef = "refs/remotes/origin/HEAD"
	// EventPush is the GitHub Actions push event name.
	eventPush = "push"
	// PayloadKeyPullRequest is the top-level key in the event payload for PR events.
	payloadKeyPullRequest = "pull_request"
	// PayloadKeyMergeGroup is the top-level key in the event payload for merge_group events.
	payloadKeyMergeGroup = "merge_group"
	// EnvGitHubBaseRef is the environment variable for the PR target branch.
	envGitHubBaseRef = "GITHUB_BASE_REF"
	// SourceDefault is the source label for default fallback resolution.
	sourceDefault = "default"
	// SourceGitHubBaseRef is the source label when resolving from GITHUB_BASE_REF.
	sourceGitHubBaseRef = "GITHUB_BASE_REF"
	// SourcePayloadBaseSHA is the source label when falling back to event.pull_request.base.sha.
	sourcePayloadBaseSHA = "event.pull_request.base.sha"
	// SourceMergeGroupBaseSHA is the source label when resolving from event.merge_group.base_sha.
	sourceMergeGroupBaseSHA = "event.merge_group.base_sha"
	// EventMergeGroup is the GitHub Actions merge_group event name.
	eventMergeGroup = "merge_group"
	// RefsHeadsPrefix is the prefix on fully-qualified branch refs in event payloads.
	refsHeadsPrefix = "refs/heads/"
)

// ErrEventPathNotSet is returned when $GITHUB_EVENT_PATH is not set.
var ErrEventPathNotSet = fmt.Errorf("GITHUB_EVENT_PATH is not set")

// ErrNoParentCommit is returned when HEAD has no parents (initial commit).
var ErrNoParentCommit = fmt.Errorf("HEAD has no parents (initial commit)")

// ResolveBase returns the base commit for affected detection in GitHub Actions.
// It reads GitHub Actions environment variables and event payloads to determine
// the appropriate base commit for the current event type.
//
// Callers running in container jobs should ensure git's safe.directory is
// configured for $GITHUB_WORKSPACE *before* invoking ResolveBase — without
// it, the merge-base auto-fetch and HEAD~1 lookups will fail with
// "dubious ownership in repository". The cmd-layer wrapper
// `internal/exec.resolveBaseFromCI` does this via
// `git.EnsureGitSafeDirectory()` and is gated by `ci.enabled`.
func (p *Provider) ResolveBase() (*provider.BaseResolution, error) {
	defer perf.Track(nil, "github.Provider.ResolveBase")()

	eventName := os.Getenv("GITHUB_EVENT_NAME")

	switch eventName {
	case "pull_request", "pull_request_target":
		return resolvePRBase(eventName)
	case eventPush:
		return resolvePushBase()
	case eventMergeGroup:
		return resolveMergeGroupBase(), nil
	default:
		return &provider.BaseResolution{
			Ref:       defaultRef,
			Source:    sourceDefault,
			EventType: eventName,
		}, nil
	}
}

// resolvePRBase resolves the base commit for pull request events.
//
// Strategy chain (first success wins):
//  1. merge-base(HEAD, origin/<target>) — gold standard. Self-heals from
//     shallow CI checkouts via MergeBaseWithAutoFetch (fetches the target
//     branch and deepens history when needed).
//  2. HEAD~1 — only for closed/merged PRs when merge-base is unavailable.
//     Correct when the merge commit is checked out with merge/squash
//     strategy.
//  3. event.pull_request.base.sha — payload SHA. Slightly stale (frozen at
//     last sync event) but never compares to the current tip of main, so it
//     can never produce the "PR is out of date with main" false positives
//     that returning the origin/<target> ref directly does.
//  4. refs/remotes/origin/<target> ref — last resort, with a Warn log.
//     Compares to current tip of target; will produce false positives for
//     out-of-date PRs.
//
// Also extracts the PR head SHA for Atmos Pro upload correlation.
func resolvePRBase(eventName string) (*provider.BaseResolution, error) {
	payload, err := readEventPayload()
	if err != nil {
		return nil, fmt.Errorf("reading GitHub event payload: %w", err)
	}

	headSHA := extractPRHeadSHA(payload)
	targetBranch := extractTargetBranch(payload)
	action, _ := payload["action"].(string)

	// 1) merge-base — the gold standard. Works regardless of what's
	// checked out, merge strategy, or number of commits on the PR.
	if targetBranch != "" {
		if sha, mbErr := git.MergeBaseWithAutoFetch(".", targetBranch); mbErr == nil {
			return &provider.BaseResolution{
				SHA:          sha,
				HeadSHA:      headSHA,
				TargetBranch: targetBranch,
				Source:       "merge-base(HEAD, origin/" + targetBranch + ")",
				EventType:    eventName,
			}, nil
		} else {
			log.Debug("merge-base failed, trying fallbacks", "target", targetBranch, "error", mbErr)
		}
	}

	// 2) Closed/merged PRs: HEAD~1.
	// Correct when the merge commit is checked out (merge/squash strategies).
	if action == "closed" {
		if sha, parentErr := resolveParentCommit(); parentErr == nil {
			return &provider.BaseResolution{
				SHA:          sha,
				HeadSHA:      headSHA,
				TargetBranch: targetBranch,
				Source:       "HEAD~1 (merged PR, merge-base unavailable)",
				EventType:    eventName,
			}, nil
		} else {
			log.Debug("HEAD~1 failed for merged PR", "error", parentErr)
		}
	}

	// 3) event.pull_request.base.sha — payload SHA fallback.
	// This SHA is at worst stale by however many main commits have landed
	// since the PR was last synced. Crucially, it is not the *current tip*
	// of main, so it will not silently turn a stale-but-untouched PR into
	// "every component is affected".
	if baseSHA := extractBaseSHA(payload); baseSHA != "" {
		return &provider.BaseResolution{
			SHA:          baseSHA,
			HeadSHA:      headSHA,
			TargetBranch: targetBranch,
			Source:       sourcePayloadBaseSHA,
			EventType:    eventName,
		}, nil
	}

	// 4) Last-resort: ref to current tip of target branch. Logs Warn
	// because this is the path that produces false positives for
	// out-of-date PRs (every commit on main since the fork point shows
	// up as a tree difference).
	res := resolveFromBaseRef(eventName)
	res.HeadSHA = headSHA
	res.TargetBranch = targetBranch
	log.Warn(
		"Falling back to current tip of target branch for PR base — affected detection may include unrelated commits from the target branch.",
		"target", targetBranch,
		"hint", "ensure the workflow checks out enough history (fetch-depth >= 2 or fetch-depth: 0) and that origin/"+targetBranch+" is fetchable",
	)
	return res, nil
}

// extractTargetBranch extracts the target branch name from the PR event payload.
// Falls back to GITHUB_BASE_REF environment variable if not found in the payload.
func extractTargetBranch(payload map[string]any) string {
	pr, _ := payload[payloadKeyPullRequest].(map[string]any)
	if pr == nil {
		return os.Getenv(envGitHubBaseRef)
	}

	base, _ := pr["base"].(map[string]any)
	if base == nil {
		return os.Getenv(envGitHubBaseRef)
	}

	ref, _ := base["ref"].(string)
	if ref == "" {
		return os.Getenv(envGitHubBaseRef)
	}

	return ref
}

// extractPRHeadSHA extracts the head commit SHA from a pull request event payload.
// This SHA is used for upload correlation with Atmos Pro, which indexes by head.sha.
func extractPRHeadSHA(payload map[string]any) string {
	pr, _ := payload[payloadKeyPullRequest].(map[string]any)
	if pr == nil {
		return ""
	}

	head, _ := pr["head"].(map[string]any)
	if head == nil {
		return ""
	}

	sha, _ := head["sha"].(string)
	return sha
}

// extractBaseSHA extracts the base commit SHA from a pull request event payload.
// This is the SHA of the target branch tip at the time of the PR event (open
// or last sync), and is used as the payload-base fallback when merge-base
// cannot resolve.
func extractBaseSHA(payload map[string]any) string {
	pr, _ := payload[payloadKeyPullRequest].(map[string]any)
	if pr == nil {
		return ""
	}

	base, _ := pr["base"].(map[string]any)
	if base == nil {
		return ""
	}

	sha, _ := base["sha"].(string)
	return sha
}

// resolveFromBaseRef resolves the base from $GITHUB_BASE_REF, falling back to defaultRef.
func resolveFromBaseRef(eventName string) *provider.BaseResolution {
	baseRef := os.Getenv(envGitHubBaseRef)
	if baseRef == "" {
		return &provider.BaseResolution{
			Ref:       defaultRef,
			Source:    sourceDefault + " (" + envGitHubBaseRef + " empty)",
			EventType: eventName,
		}
	}

	return &provider.BaseResolution{
		Ref:       "refs/remotes/origin/" + baseRef,
		Source:    sourceGitHubBaseRef,
		EventType: eventName,
	}
}

// resolvePushBase resolves the base commit for push events.
// For force-pushes, it falls back to HEAD~1 since the previous commit may not exist.
// For normal pushes, it uses the "before" SHA from the event payload.
func resolvePushBase() (*provider.BaseResolution, error) {
	payload, err := readEventPayload()
	if err != nil {
		return nil, fmt.Errorf("reading GitHub event payload: %w", err)
	}

	// Check for force-push — the "before" commit may not exist.
	forced, _ := payload["forced"].(bool)
	if forced {
		sha, err := resolveParentCommit()
		if err != nil {
			log.Warn("Failed to resolve HEAD~1 for force-push, falling back to origin/HEAD", "error", err)
			return &provider.BaseResolution{
				Ref:       defaultRef,
				Source:    sourceDefault + " (force-push, HEAD~1 failed)",
				EventType: eventPush,
			}, nil
		}
		return &provider.BaseResolution{
			SHA:       sha,
			Source:    "HEAD~1 (force-push)",
			EventType: eventPush,
		}, nil
	}

	// Normal push — use the "before" SHA.
	before, _ := payload["before"].(string)
	if before == "" || before == "0000000000000000000000000000000000000000" {
		// New branch push or missing before — fall back to origin/HEAD.
		return &provider.BaseResolution{
			Ref:       defaultRef,
			Source:    sourceDefault + " (no before SHA)",
			EventType: eventPush,
		}, nil
	}

	return &provider.BaseResolution{
		SHA:       before,
		Source:    "event.before",
		EventType: eventPush,
	}, nil
}

// resolveMergeGroupBase resolves the base commit for merge_group events.
//
// GitHub creates a synthetic merge commit on a temporary
// `gh-readonly-queue/<base>/pr-<N>-<sha>` branch when a PR enters the merge
// queue, and re-runs all required status checks against that new SHA. The
// event payload exposes:
//   - merge_group.base_sha — the target-branch commit the synthetic commit
//     was merged on top of (the right diff base for "affected").
//   - merge_group.head_sha — the synthetic merge commit (used for upload
//     correlation, parallel to pull_request.head.sha).
//   - merge_group.base_ref — the fully-qualified target branch ref
//     (e.g. "refs/heads/main").
//
// Strategy:
//  1. Read the event payload and use base_sha / head_sha / base_ref directly.
//  2. If the payload is missing or fields are empty (e.g. test environments
//     without a real GitHub event file), fall back to GITHUB_BASE_REF and
//     ultimately to refs/remotes/origin/HEAD.
func resolveMergeGroupBase() *provider.BaseResolution {
	payload, err := readEventPayload()
	if err != nil {
		// Without a payload we cannot read merge_group.base_sha — fall back
		// to env-only resolution rather than failing the whole describe-affected
		// run. This preserves test-environment ergonomics.
		log.Debug("merge_group: event payload unavailable, falling back to GITHUB_BASE_REF", "error", err)
		return resolveFromBaseRef(eventMergeGroup)
	}

	baseSHA := extractMergeGroupBaseSHA(payload)
	headSHA := extractMergeGroupHeadSHA(payload)
	targetBranch := extractMergeGroupTargetBranch(payload)

	if baseSHA != "" {
		return &provider.BaseResolution{
			SHA:          baseSHA,
			HeadSHA:      headSHA,
			TargetBranch: targetBranch,
			Source:       sourceMergeGroupBaseSHA,
			EventType:    eventMergeGroup,
		}
	}

	// Payload is present but lacks merge_group.base_sha — last-resort env fallback.
	res := resolveFromBaseRef(eventMergeGroup)
	if headSHA != "" {
		res.HeadSHA = headSHA
	}
	if targetBranch != "" {
		res.TargetBranch = targetBranch
	}
	return res
}

// extractMergeGroupBaseSHA extracts merge_group.base_sha from the event payload.
// Returns empty string if absent.
func extractMergeGroupBaseSHA(payload map[string]any) string {
	mg, _ := payload[payloadKeyMergeGroup].(map[string]any)
	if mg == nil {
		return ""
	}
	sha, _ := mg["base_sha"].(string)
	return sha
}

// extractMergeGroupHeadSHA extracts merge_group.head_sha from the event payload.
// This is the synthetic merge commit SHA Atmos Pro indexes by; used for upload
// correlation. Returns empty string if absent.
func extractMergeGroupHeadSHA(payload map[string]any) string {
	mg, _ := payload[payloadKeyMergeGroup].(map[string]any)
	if mg == nil {
		return ""
	}
	sha, _ := mg["head_sha"].(string)
	return sha
}

// extractMergeGroupTargetBranch extracts the target branch name from
// merge_group.base_ref (e.g. "refs/heads/main" → "main"). Falls back to
// $GITHUB_BASE_REF, then empty.
func extractMergeGroupTargetBranch(payload map[string]any) string {
	mg, _ := payload[payloadKeyMergeGroup].(map[string]any)
	if mg != nil {
		ref, _ := mg["base_ref"].(string)
		if ref != "" {
			return strings.TrimPrefix(ref, refsHeadsPrefix)
		}
	}
	return os.Getenv(envGitHubBaseRef)
}

// readEventPayload reads and parses the GitHub event payload from $GITHUB_EVENT_PATH.
func readEventPayload() (map[string]any, error) {
	eventPath := os.Getenv("GITHUB_EVENT_PATH")
	if eventPath == "" {
		return nil, ErrEventPathNotSet
	}

	// Clean the path to normalize it.
	eventPath = filepath.Clean(eventPath)

	data, err := os.ReadFile(eventPath) //nolint:gosec // G703: Path is from trusted $GITHUB_EVENT_PATH env var set by GitHub Actions runner.
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", eventPath, err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("parsing event payload: %w", err)
	}

	return payload, nil
}

// resolveParentCommit resolves HEAD~1 using git.
func resolveParentCommit() (string, error) {
	repo, err := git.GetLocalRepo()
	if err != nil {
		return "", fmt.Errorf("opening local repo: %w", err)
	}

	head, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("getting HEAD: %w", err)
	}

	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return "", fmt.Errorf("getting HEAD commit: %w", err)
	}

	if commit.NumParents() == 0 {
		return "", ErrNoParentCommit
	}

	parent, err := commit.Parent(0)
	if err != nil {
		return "", fmt.Errorf("getting parent commit: %w", err)
	}

	return parent.Hash.String(), nil
}
