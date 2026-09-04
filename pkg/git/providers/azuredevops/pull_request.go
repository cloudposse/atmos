// Package azuredevops implements Azure DevOps' pull-request API as a git publishing provider.
//
// Azure DevOps addresses a repository with three segments -- organization, project, and
// repository -- rather than GitHub's two (owner, repository). It maps onto the shared,
// forge-neutral atmosgit.PullRequestOptions as: Owner is the organization, Namespace is exactly
// one element (the project), and Repository is the repository name.
package azuredevops

import (
	"context"
	"fmt"
	"os"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	pkghttp "github.com/cloudposse/atmos/pkg/http"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ProviderName identifies the Azure DevOps pull request publisher.
const ProviderName = "azuredevops"

// defaultBaseURL is Azure DevOps' cloud (dev.azure.com) organization host.
const defaultBaseURL = "https://dev.azure.com"

func init() {
	atmosgit.RegisterPullRequestPublisher(ProviderName, func() (atmosgit.PullRequestPublisher, error) {
		return New(), nil
	})
}

// Provider reconciles pull requests through Azure DevOps' REST API.
type Provider struct {
	client   pkghttp.Client
	baseURL  string
	tokenEnv func() (string, error)
}

// Option configures a Provider (functional options pattern).
type Option func(*Provider)

// WithHTTPClient overrides the HTTP client used to call the Azure DevOps REST API. Primarily
// useful for tests, which point it at an httptest.Server (a *http.Client already satisfies
// pkghttp.Client).
func WithHTTPClient(c pkghttp.Client) Option {
	return func(p *Provider) { p.client = c }
}

// WithBaseURL overrides Azure DevOps' organization host, e.g. for tests or for Azure DevOps
// Server (on-premises) deployments that don't live under dev.azure.com.
func WithBaseURL(baseURL string) Option {
	return func(p *Provider) { p.baseURL = strings.TrimSuffix(baseURL, "/") }
}

// WithToken overrides how the provider's personal access token is resolved. Primarily useful for
// tests, which would otherwise need to mutate the process-wide AZURE_DEVOPS_EXT_PAT environment
// variable (a source of test flakiness/races under `go test -parallel`).
func WithToken(token string) Option {
	return func(p *Provider) { p.tokenEnv = func() (string, error) { return token, nil } }
}

// New creates an Azure DevOps pull request provider authenticated with AZURE_DEVOPS_EXT_PAT.
func New(opts ...Option) *Provider {
	p := &Provider{
		client:   pkghttp.NewDefaultClient(),
		baseURL:  defaultBaseURL,
		tokenEnv: tokenFromEnv,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// tokenFromEnv reads the Azure DevOps personal access token from AZURE_DEVOPS_EXT_PAT, the
// environment variable this org's other Azure DevOps tooling (ado.sh's ensure_pr/inject_pat_url)
// already standardizes on, used as HTTP Basic auth with an empty username.
func tokenFromEnv() (string, error) {
	defer perf.Track(nil, "azuredevops.tokenFromEnv")()

	//nolint:forbidigo // Direct env lookup mirrors githubci.NewClient's own token resolution.
	token := os.Getenv("AZURE_DEVOPS_EXT_PAT")
	if token == "" {
		return "", errUtils.ErrAzureDevOpsTokenNotFound
	}
	return token, nil
}

// Reconcile creates or updates a pull request and applies configured metadata.
func (p *Provider) Reconcile(ctx context.Context, options *atmosgit.PullRequestOptions) (*atmosgit.PullRequestResult, error) {
	defer perf.Track(nil, "azuredevops.Provider.Reconcile")()

	if err := validateOptions(options); err != nil {
		return nil, err
	}
	project, err := requireProjectNamespace(options.Namespace)
	if err != nil {
		return nil, err
	}
	token, err := p.tokenEnv()
	if err != nil {
		return nil, fmt.Errorf("%w: configure AZURE_DEVOPS_EXT_PAT: %w", errUtils.ErrAzureDevOpsAuthorization, err)
	}

	repo := repositoryEndpoint{baseURL: p.baseURL, organization: options.Owner, project: project, repository: options.Repository}
	result, pr, err := p.reconcilePullRequest(ctx, repo, token, options)
	if err != nil {
		return nil, err
	}

	if err := p.applyMetadata(ctx, repo, token, pr.PullRequestID, options); err != nil {
		return result, err
	}
	return result, nil
}

// reconcilePullRequest finds an existing active pull request for options' source/target branches
// and updates it in place, or creates a new one when none is found.
func (p *Provider) reconcilePullRequest(ctx context.Context, repo repositoryEndpoint, token string, options *atmosgit.PullRequestOptions) (*atmosgit.PullRequestResult, *pullRequest, error) {
	defer perf.Track(nil, "azuredevops.Provider.reconcilePullRequest")()

	existing, err := p.findActivePullRequest(ctx, repo, token, options.Base, options.Head)
	if err != nil {
		return nil, nil, err
	}

	var pr *pullRequest
	created := false
	if existing != nil {
		pr, err = p.updatePullRequest(ctx, repo, token, existing.PullRequestID, updatePullRequestBody{Title: options.Title, Description: options.Body})
	} else {
		pr, err = p.createPullRequest(ctx, repo, token, options)
		created = true
	}
	if err != nil {
		return nil, nil, err
	}

	result := &atmosgit.PullRequestResult{Number: pr.PullRequestID, URL: repo.pullRequestWebURL(pr.PullRequestID), Created: created}
	return result, pr, nil
}

// applyMetadata applies labels and reviewers to prID, and rejects assignees outright since Azure
// DevOps pull requests don't support them (failing loudly rather than silently dropping the
// configured value).
func (p *Provider) applyMetadata(ctx context.Context, repo repositoryEndpoint, token string, prID int, options *atmosgit.PullRequestOptions) error {
	defer perf.Track(nil, "azuredevops.Provider.applyMetadata")()

	if len(options.Assignees) > 0 {
		return fmt.Errorf("%w: configure reviewers instead", errUtils.ErrAzureDevOpsAssigneesUnsupported)
	}
	for _, label := range options.Labels {
		if err := p.addLabel(ctx, repo, token, prID, label); err != nil {
			return err
		}
	}
	for _, reviewer := range options.Reviewers {
		if err := p.addReviewer(ctx, repo, token, prID, reviewer); err != nil {
			return err
		}
	}
	return nil
}

// validateOptions mirrors the GitHub provider's own required-field contract.
func validateOptions(options *atmosgit.PullRequestOptions) error {
	if options == nil {
		return fmt.Errorf("%w: pull request options are required", errUtils.ErrComponentUpdaterConfig)
	}
	if options.Owner == "" || options.Repository == "" || options.Base == "" || options.Head == "" {
		return fmt.Errorf("%w: owner, repository, base, and head are required", errUtils.ErrComponentUpdaterConfig)
	}
	return nil
}

// requireProjectNamespace enforces Azure DevOps' three-segment addressing: exactly one namespace
// element (the project), non-empty.
func requireProjectNamespace(namespace []string) (string, error) {
	if len(namespace) != 1 || namespace[0] == "" {
		return "", fmt.Errorf("%w: got %v", errUtils.ErrAzureDevOpsNamespaceInvalid, namespace)
	}
	return namespace[0], nil
}
