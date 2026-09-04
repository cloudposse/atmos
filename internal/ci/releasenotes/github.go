package releasenotes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// errGitHubRequestFailed wraps any non-2xx response from the releases API,
// carrying the status and body as %w-wrapped context rather than a dynamic
// error string.
var errGitHubRequestFailed = errors.New("releasenotes: GitHub API request failed")

// ReleaseRef identifies one GitHub release to read or update: Repo in
// "owner/name" form, ID the release's numeric ID (as returned by the
// releases API) formatted as a string.
type ReleaseRef struct {
	Repo string
	ID   string
}

// GetReleaseBody fetches ref's current release body via the REST API.
func GetReleaseBody(ctx context.Context, client HTTPClient, token string, ref ReleaseRef) (string, error) {
	defer perf.Track(nil, "releasenotes.GetReleaseBody")()

	req, err := newGitHubRequest(ctx, http.MethodGet, token, ref, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("releasenotes: get release %s: %w", ref.ID, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("releasenotes: read release %s response: %w", ref.ID, err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: get release %s returned %s: %s", errGitHubRequestFailed, ref.ID, resp.Status, string(body))
	}

	var release struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return "", fmt.Errorf("releasenotes: decode release %s: %w", ref.ID, err)
	}
	return release.Body, nil
}

// UpdateReleaseBody sets ref's release body. Every other field is left
// untouched (tag_name, draft, prerelease, ...), since the API only updates
// fields present in the request payload.
func UpdateReleaseBody(ctx context.Context, client HTTPClient, token string, ref ReleaseRef, body string) error {
	defer perf.Track(nil, "releasenotes.UpdateReleaseBody")()

	payload, err := json.Marshal(struct {
		Body string `json:"body"`
	}{Body: body})
	if err != nil {
		return fmt.Errorf("releasenotes: encode release %s update: %w", ref.ID, err)
	}

	req, err := newGitHubRequest(ctx, http.MethodPatch, token, ref, strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("releasenotes: update release %s: %w", ref.ID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%w: update release %s returned %s: %s", errGitHubRequestFailed, ref.ID, resp.Status, string(respBody))
	}
	return nil
}

func newGitHubRequest(ctx context.Context, method, token string, ref ReleaseRef, body io.Reader) (*http.Request, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/%s", ref.Repo, ref.ID)
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("releasenotes: build %s request for release %s: %w", method, ref.ID, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}
