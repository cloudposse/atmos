package azuredevops

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
)

// apiVersion pins the Azure DevOps Git REST API version this client speaks.
const apiVersion = "7.1"

// activeStatus is the pull request status Azure DevOps' searchCriteria.status filters on.
const activeStatus = "active"

// wrapErrFmt wraps a lower-level error under a sentinel from errors/errors.go, per CLAUDE.md's
// error-handling convention (a named constant since golangci-lint's revive add-constant flags
// repeated literal format strings).
const wrapErrFmt = "%w: %w"

// restRequest bundles one Azure DevOps REST call's inputs (Options Pattern-adjacent: CLAUDE.md's
// revive argument-limit caps functions at 5 positional parameters; method/url/token/body are each
// independently meaningful per call, so they're bundled here instead of trimmed).
type restRequest struct {
	method string
	url    string
	token  string
	body   any
}

// pullRequest is the subset of Azure DevOps' GitPullRequest resource this provider reads.
type pullRequest struct {
	PullRequestID int    `json:"pullRequestId"`
	Title         string `json:"title"`
	Description   string `json:"description"`
}

// pullRequestListResponse wraps Azure DevOps' collection response envelope.
type pullRequestListResponse struct {
	Value []pullRequest `json:"value"`
}

// createPullRequestBody is the payload for POST .../pullrequests.
type createPullRequestBody struct {
	SourceRefName string `json:"sourceRefName"`
	TargetRefName string `json:"targetRefName"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	IsDraft       bool   `json:"isDraft"`
}

// updatePullRequestBody is the payload for PATCH .../pullrequests/{id}.
type updatePullRequestBody struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// labelBody is the payload for POST .../pullrequests/{id}/labels.
type labelBody struct {
	Name string `json:"name"`
}

// reviewerBody is the payload for PUT .../pullrequests/{id}/reviewers/{reviewerID}.
type reviewerBody struct {
	ID string `json:"id"`
}

// repositoryEndpoint addresses one Azure DevOps Git repository and builds the REST/web URLs for
// it.
type repositoryEndpoint struct {
	baseURL      string
	organization string
	project      string
	repository   string
}

// apiBase returns the repository's `_apis/git/repositories/{repository}` REST root.
func (r repositoryEndpoint) apiBase() string {
	return fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s",
		r.baseURL, url.PathEscape(r.organization), url.PathEscape(r.project), url.PathEscape(r.repository))
}

// pullRequestWebURL builds the browser-facing URL for pull request id, since Azure DevOps' REST
// responses don't include one directly.
func (r repositoryEndpoint) pullRequestWebURL(id int) string {
	return fmt.Sprintf("%s/%s/_git/%s/pullrequest/%d",
		r.baseURL, url.PathEscape(r.organization), url.PathEscape(r.repository), id)
}

// refName qualifies branch as a full Git ref, as Azure DevOps' PR API requires.
func refName(branch string) string {
	return "refs/heads/" + branch
}

// findActivePullRequest looks up an active pull request from source to target, returning nil
// (with no error) when none exists yet.
func (p *Provider) findActivePullRequest(ctx context.Context, repo repositoryEndpoint, token, base, head string) (*pullRequest, error) {
	defer perf.Track(nil, "azuredevops.Provider.findActivePullRequest")()

	query := url.Values{}
	query.Set("searchCriteria.sourceRefName", refName(head))
	query.Set("searchCriteria.targetRefName", refName(base))
	query.Set("searchCriteria.status", activeStatus)
	query.Set("api-version", apiVersion)

	var list pullRequestListResponse
	req := restRequest{method: http.MethodGet, url: repo.apiBase() + "/pullrequests?" + query.Encode(), token: token}
	if err := p.doRequest(ctx, req, &list); err != nil {
		return nil, err
	}
	if len(list.Value) == 0 {
		return nil, nil
	}
	return &list.Value[0], nil
}

// createPullRequest opens a new pull request from options' head to base.
func (p *Provider) createPullRequest(ctx context.Context, repo repositoryEndpoint, token string, options *atmosgit.PullRequestOptions) (*pullRequest, error) {
	defer perf.Track(nil, "azuredevops.Provider.createPullRequest")()

	body := createPullRequestBody{
		SourceRefName: refName(options.Head),
		TargetRefName: refName(options.Base),
		Title:         options.Title,
		Description:   options.Body,
		IsDraft:       options.Draft,
	}
	var pr pullRequest
	requestURL := fmt.Sprintf("%s/pullrequests?api-version=%s", repo.apiBase(), apiVersion)
	if err := p.doRequest(ctx, restRequest{method: http.MethodPost, url: requestURL, token: token, body: body}, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// updatePullRequest refreshes title/description on an existing pull request.
func (p *Provider) updatePullRequest(ctx context.Context, repo repositoryEndpoint, token string, id int, payload updatePullRequestBody) (*pullRequest, error) {
	defer perf.Track(nil, "azuredevops.Provider.updatePullRequest")()

	var pr pullRequest
	requestURL := fmt.Sprintf("%s/pullrequests/%d?api-version=%s", repo.apiBase(), id, apiVersion)
	if err := p.doRequest(ctx, restRequest{method: http.MethodPatch, url: requestURL, token: token, body: payload}, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// addLabel tags pull request id with label.
func (p *Provider) addLabel(ctx context.Context, repo repositoryEndpoint, token string, id int, label string) error {
	defer perf.Track(nil, "azuredevops.Provider.addLabel")()

	requestURL := fmt.Sprintf("%s/pullrequests/%d/labels?api-version=%s-preview.1", repo.apiBase(), id, apiVersion)
	return p.doRequest(ctx, restRequest{method: http.MethodPost, url: requestURL, token: token, body: labelBody{Name: label}}, nil)
}

// addReviewer requests reviewerID (an Azure DevOps identity descriptor or GUID -- Azure DevOps'
// reviewers API, unlike GitHub's, does not accept a plain username) as a reviewer on pull request
// id.
func (p *Provider) addReviewer(ctx context.Context, repo repositoryEndpoint, token string, id int, reviewerID string) error {
	defer perf.Track(nil, "azuredevops.Provider.addReviewer")()

	requestURL := fmt.Sprintf("%s/pullrequests/%d/reviewers/%s?api-version=%s", repo.apiBase(), id, url.PathEscape(reviewerID), apiVersion)
	return p.doRequest(ctx, restRequest{method: http.MethodPut, url: requestURL, token: token, body: reviewerBody{ID: reviewerID}}, nil)
}

// doRequest issues an authenticated REST call and decodes a JSON response into out (when
// non-nil), translating HTTP failures into actionable, sentinel-wrapped errors.
func (p *Provider) doRequest(ctx context.Context, r restRequest, out any) error {
	defer perf.Track(nil, "azuredevops.Provider.doRequest")()

	var reader io.Reader
	if r.body != nil {
		encoded, err := json.Marshal(r.body)
		if err != nil {
			return fmt.Errorf(wrapErrFmt, errUtils.ErrPullRequestReconciliation, err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, r.method, r.url, reader)
	if err != nil {
		return fmt.Errorf(wrapErrFmt, errUtils.ErrPullRequestReconciliation, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	// Azure DevOps' PAT auth is HTTP Basic with an empty username and the PAT as the password.
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(":"+r.token)))

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf(wrapErrFmt, errUtils.ErrPullRequestReconciliation, err)
	}
	defer resp.Body.Close()

	return decodeResponse(resp, out)
}

// decodeResponse validates resp's status code and decodes its JSON body into out when non-nil.
func decodeResponse(resp *http.Response, out any) error {
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("%w: azure DevOps returned HTTP %d; verify AZURE_DEVOPS_EXT_PAT has Code (read & write) and Pull Request Contribute scopes",
			errUtils.ErrAzureDevOpsAuthorization, resp.StatusCode)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		payload, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%w: azure DevOps returned HTTP %d: %s", errUtils.ErrPullRequestReconciliation, resp.StatusCode, string(payload))
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf(wrapErrFmt, errUtils.ErrPullRequestReconciliation, err)
	}
	return nil
}
