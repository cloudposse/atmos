package github

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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
	// EnvGitHubBaseRef is the environment variable for the PR target branch.
	envGitHubBaseRef = "GITHUB_BASE_REF"
	// SourceDefault is the source label for default fallback resolution.
	sourceDefault = "default"
	// SourceGitHubBaseRef is the source label when resolving from GITHUB_BASE_REF.
	sourceGitHubBaseRef = "GITHUB_BASE_REF"
)

// ErrEventPathNotSet is returned when $GITHUB_EVENT_PATH is not set.
var ErrEventPathNotSet = fmt.Errorf("GITHUB_EVENT_PATH is not set")

// ErrNoParentCommit is returned when HEAD has no parents (initial commit).
var ErrNoParentCommit = fmt.Errorf("HEAD has no parents (initial commit)")

// ResolveBase returns the base commit for affected detection in GitHub Actions.
// It reads GitHub Actions environment variables and event payloads to determine
// the appropriate base commit for the current event type.
func (p *Provider) ResolveBase() (*provider.BaseResolution, error) {
	defer perf.Track(nil, "github.Provider.ResolveBase")()

	eventName := os.Getenv("GITHUB_EVENT_NAME")

	switch eventName {
	case "pull_request", "pull_request_target":
		return resolvePRBase(eventName)
	case eventPush:
		return resolvePushBase()
	case "merge_group":
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
// For closed/merged PRs, it uses HEAD~1 (the parent of the merge commit).
// For open/synchronize PRs, it computes the merge-base with the target branch.
func resolvePRBase(eventName string) (*provider.BaseResolution, error) {
	payload, err := readEventPayload()
	if err != nil {
		return nil, fmt.Errorf("reading GitHub event payload: %w", err)
	}

	// Check if this is a closed PR (merged).
	action, _ := payload["action"].(string)
	if action == "closed" {
		// For merged PRs, the workflow should check out the merge commit.
		// The base is its first parent (the pre-merge state of the target branch).
		// Using HEAD~1 is always correct regardless of merge strategy (merge, squash, rebase),
		// unlike pull_request.base.sha which can be stale if other PRs merged first.
		sha, err := resolveParentCommit()
		if err != nil {
			log.Warn("Failed to resolve HEAD~1 for merged PR, falling back to GITHUB_BASE_REF", "error", err)
		} else {
			return &provider.BaseResolution{
				SHA:       sha,
				Source:    "HEAD~1 (merged PR)",
				EventType: eventName,
			}, nil
		}
	}

	// For open/synchronize PRs, use the target branch ref.
	return resolveFromBaseRef(eventName), nil
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

// resolveMergeGroupBase resolves the base commit for merge group events.
func resolveMergeGroupBase() *provider.BaseResolution {
	return resolveFromBaseRef("merge_group")
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
