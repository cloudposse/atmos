package github

import (
	"os"
	"strconv"
	"strings"

	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	ghtoken "github.com/cloudposse/atmos/pkg/github"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// Keys and sides of the pull_request and workflow_run event payload objects.
const (
	prSideHead = "head"
	prSideBase = "base"

	payloadKeyWorkflowRun = "workflow_run"
	payloadKeyNumber      = "number"
	payloadKeyFullName    = "full_name"
)

// pullRequestFromEvent builds the pull request description for a pull_request or
// pull_request_target event. The number comes from the event payload, because GitHub runs
// pull_request_target (and workflow_run) against the base branch and so sets GITHUB_REF to
// refs/heads/<base> rather than refs/pull/<n>/merge. GITHUB_REF is the fallback.
func pullRequestFromEvent() *provider.PRInfo {
	payload, err := readEventPayload()
	if err != nil {
		log.Warn("Cannot read the event payload, treating the pull request as coming from a fork", "error", err)
		payload = nil
	}

	pr, _ := payload[payloadKeyPullRequest].(map[string]any)
	prNumber := payloadNumber(pr)
	if prNumber == 0 {
		prNumber = numberFromEnv()
	}

	repo := os.Getenv("GITHUB_REPOSITORY")
	var prURL string
	if prNumber > 0 && repo != "" {
		prURL = ghtoken.RepoEndpoints().ServerURL + "/" + repo + "/pull/" + strconv.Itoa(prNumber)
	}

	return &provider.PRInfo{
		Number:  prNumber,
		HeadRef: os.Getenv("GITHUB_HEAD_REF"),
		BaseRef: os.Getenv("GITHUB_BASE_REF"),
		URL:     prURL,
		Fork:    prHeadIsFork(pr, repo),
	}
}

// numberFromEnv extracts the pull request number from GITHUB_REF (refs/pull/<n>/merge) or,
// failing that, from GITHUB_REF_NAME (<n>/merge). It returns 0 when neither carries one.
func numberFromEnv() int {
	if ref := os.Getenv("GITHUB_REF"); strings.HasPrefix(ref, "refs/pull/") {
		if parts := strings.Split(ref, "/"); len(parts) >= 3 {
			if n, err := strconv.Atoi(parts[2]); err == nil {
				return n
			}
		}
	}
	if refName := os.Getenv("GITHUB_REF_NAME"); strings.HasSuffix(refName, "/merge") {
		if n, err := strconv.Atoi(strings.TrimSuffix(refName, "/merge")); err == nil {
			return n
		}
	}
	return 0
}

// payloadNumber returns the integer "number" of a payload object, or 0 when absent.
func payloadNumber(obj map[string]any) int {
	n, ok := obj[payloadKeyNumber].(float64)
	if !ok || n < 0 {
		return 0
	}
	return int(n)
}

// workflowRunPullRequest describes the pull request behind a workflow_run event. It returns nil
// for a same-repository run that names no pull request. A run from a fork, a deleted fork, or an
// unreadable payload is described as a fork pull request so the posting gate holds it instead of
// failing open. GitHub leaves workflow_run.pull_requests empty for fork runs, so the number is 0
// unless the payload supplies one.
func workflowRunPullRequest(repository string) *provider.PRInfo {
	payload, err := readEventPayload()
	if err != nil {
		log.Warn("Cannot read the event payload, treating the workflow run as coming from a fork", "error", err)
		return &provider.PRInfo{Fork: true}
	}

	run, _ := payload[payloadKeyWorkflowRun].(map[string]any)
	if repository == "" {
		repository = payloadRepositoryName(payload)
	}

	headRepo, _ := run["head_repository"].(map[string]any)
	fork := !sameRepository(headRepo, repository)
	if headRepo == nil {
		log.Warn("The workflow run has no head repository (deleted fork), treating it as a fork run")
	}

	info := &provider.PRInfo{Fork: fork}
	info.HeadRef, _ = run["head_branch"].(string)
	if prs, _ := run["pull_requests"].([]any); len(prs) > 0 {
		first, _ := prs[0].(map[string]any)
		info.Number = payloadNumber(first)
		base, _ := first["base"].(map[string]any)
		info.BaseRef, _ = base["ref"].(string)
	}
	if !fork && info.Number == 0 {
		return nil
	}
	if info.Number > 0 && repository != "" {
		info.URL = ghtoken.RepoEndpoints().ServerURL + "/" + repository + "/pull/" + strconv.Itoa(info.Number)
	}
	return info
}

// payloadRepositoryName returns repository.full_name from the event payload.
func payloadRepositoryName(payload map[string]any) string {
	repo, _ := payload["repository"].(map[string]any)
	name, _ := repo[payloadKeyFullName].(string)
	return name
}

// prHeadIsFork reports whether the pull_request object's head lives in a different repository
// than its base. Only the repository full names are compared: head.repo.fork says the head
// repository is a fork of something, which holds for every pull request opened inside a
// repository that is itself a fork. A deleted fork (head.repo is null), a missing pull_request
// object, or missing names cannot be proven same-repository and so count as a fork.
// The defaultBase argument names the base repository when the payload omits base.repo.
func prHeadIsFork(pr map[string]any, defaultBase string) bool {
	headRepo := nestedRepo(pr, prSideHead)
	if headRepo == nil {
		log.Warn("The pull request has no head repository (deleted fork or unreadable payload), treating it as a fork")
		return true
	}
	baseName, _ := nestedRepo(pr, prSideBase)[payloadKeyFullName].(string)
	if baseName == "" {
		baseName = defaultBase
	}
	return !sameRepository(headRepo, baseName)
}

// sameRepository reports whether the payload repository object names the repository base.
// A nil object, or an empty name on either side, is not the same repository.
func sameRepository(repo map[string]any, base string) bool {
	name, _ := repo[payloadKeyFullName].(string)
	return name != "" && base != "" && strings.EqualFold(name, base)
}

// nestedRepo returns pr[side].repo, or nil when any level is missing.
func nestedRepo(pr map[string]any, side string) map[string]any {
	sideMap, _ := pr[side].(map[string]any)
	repo, _ := sideMap["repo"].(map[string]any)
	return repo
}

// mergeWorkflowRunPullRequest combines a pull request that is already known with the one the
// workflow_run payload describes. The known pull request is kept, and only gains the fork flag
// when the payload proves the run came from a fork.
func mergeWorkflowRunPullRequest(known, fromRun *provider.PRInfo) *provider.PRInfo {
	if known == nil {
		return fromRun
	}
	if fromRun != nil && fromRun.Fork {
		known.Fork = true
	}
	return known
}
