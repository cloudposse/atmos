package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/google/go-github/v59/github"

	"github.com/cloudposse/atmos/pkg/perf"
)

// ErrPathNotFound indicates the requested directory does not exist at the requested ref.
var ErrPathNotFound = errors.New("path not found")

// treeQuery names one directory of one repository at one ref.
type treeQuery struct {
	owner   string
	repo    string
	ref     string
	dirPath string
}

// TreeSHA returns the Git tree SHA of the directory at dirPath in owner/repo at ref. Two
// directories with the same tree SHA have byte-identical contents, so comparing them proves a
// directory moved between repositories without changing. The path must name a directory below
// the repository root.
//
// The SHA is read from the parent directory listing, which carries the object SHA of every entry,
// so each lookup costs one request. A missing ref wraps ErrRefNotFound and a missing directory
// wraps ErrPathNotFound; any other failure is returned as the shared client reports it
// (rate limiting and rejected credentials included).
//
// The repositories compared are public, so when GITHUB_SERVER_URL points at a GitHub Enterprise
// Server the lookup still targets public github.com, without sending the server-scoped token.
func TreeSHA(ctx context.Context, owner, repo, ref, dirPath string) (string, error) {
	defer perf.Track(nil, "github.TreeSHA")()

	endpoints := RepoEndpoints()
	token := GetGitHubTokenContext(ctx)
	if !endpoints.isDefaultGitHubCom() {
		endpoints = publicGitHubEndpoints
		token = ""
	}
	client, _ := newGitHubClientForEndpoints(ctx, token, endpoints)
	return treeSHA(ctx, client, &treeQuery{owner: owner, repo: repo, ref: ref, dirPath: dirPath})
}

// treeSHA is the client-injectable core of TreeSHA, split out so tests can use a mock server.
func treeSHA(ctx context.Context, client *github.Client, q *treeQuery) (string, error) {
	defer perf.Track(nil, "github.treeSHA")()

	if q.dirPath == "" || q.dirPath == "." {
		return "", fmt.Errorf("%w: a directory path is required", ErrPathNotFound)
	}
	parent, name := path.Split(path.Clean(q.dirPath))
	parent = strings.TrimSuffix(parent, "/")

	_, entries, resp, err := client.Repositories.GetContents(ctx, q.owner, q.repo, parent, &github.RepositoryContentGetOptions{Ref: q.ref})
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return "", missingTree(ctx, client, q)
		}
		return "", handleGitHubAPIError(err, resp)
	}
	for _, entry := range entries {
		if entry.GetName() != name {
			continue
		}
		if entry.GetType() != "dir" {
			return "", fmt.Errorf("%w: %s/%s has %s at %s, which is not a directory", ErrPathNotFound, q.owner, q.repo, q.dirPath, q.ref)
		}
		return entry.GetSHA(), nil
	}
	return "", fmt.Errorf("%w: %s/%s has no %s at %s", ErrPathNotFound, q.owner, q.repo, q.dirPath, q.ref)
}

// missingTree explains a 404 listing a directory: either the ref or the directory is missing.
func missingTree(ctx context.Context, client *github.Client, q *treeQuery) error {
	_, resp, err := client.Repositories.GetCommitSHA1(ctx, q.owner, q.repo, q.ref, "")
	if err != nil {
		if resp != nil && (resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusUnprocessableEntity) {
			return fmt.Errorf("%w: %s/%s has no %s", ErrRefNotFound, q.owner, q.repo, q.ref)
		}
		return handleGitHubAPIError(err, resp)
	}
	return fmt.Errorf("%w: %s/%s has no %s at %s", ErrPathNotFound, q.owner, q.repo, q.dirPath, q.ref)
}
