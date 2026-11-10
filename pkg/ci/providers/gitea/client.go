package gitea

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	apiTimeout       = 30 * time.Second
	maxResponseBytes = 4 << 20
	pageSize         = 50
	// The comment pagination cap: 200 pages of 50 is 10,000 comments.
	maxCommentPages = 200
)

func apiURL() (*url.URL, error) {
	raw := configuredAPIURL()
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		!allowedScheme(u.Scheme) {
		return nil, fmt.Errorf("%w: Gitea requires an HTTPS API URL (HTTP requires ATMOS_CI_GITEA_ALLOW_INSECURE=true)", errUtils.ErrInvalidURL)
	}
	return u, nil
}

// ValidateConfig validates the endpoint before CI writes begin.
func (p *Provider) ValidateConfig() error {
	defer perf.Track(nil, "gitea.ValidateConfig")()
	_, err := apiURL()
	return err
}

func (p *Provider) request(ctx context.Context, method, path string, body, result any) error {
	base, err := apiURL()
	if err != nil {
		return err
	}
	token, err := apiToken()
	if err != nil {
		return err
	}
	var input bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&input).Encode(body); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(base.String(), "/")+path, &input)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: apiTimeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrGiteaAPIFailed, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%w: %s (HTTP %d)", errUtils.ErrGiteaAPIFailed, method, resp.StatusCode)
	}
	if result == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(result)
}

func repoPath(owner, repo string) string {
	return "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo)
}

func configuredAPIURL() string {
	raw := os.Getenv("ATMOS_CI_GITEA_API_URL")
	if raw == "" {
		raw = env("API_URL")
	}
	if raw == "" && env("SERVER_URL") != "" {
		raw = strings.TrimRight(env("SERVER_URL"), "/") + "/api/v1"
	}
	return raw
}

func allowedScheme(scheme string) bool {
	return scheme == "https" || (scheme == "http" && os.Getenv("ATMOS_CI_GITEA_ALLOW_INSECURE") == "true")
}

func apiToken() (string, error) {
	token := os.Getenv("ATMOS_CI_GITEA_TOKEN")
	if token == "" {
		token = os.Getenv("GITEA_TOKEN")
	}
	if token == "" {
		return "", fmt.Errorf("%w: Gitea API requires ATMOS_CI_GITEA_TOKEN or GITEA_TOKEN", errUtils.ErrGiteaTokenNotFound)
	}
	return token, nil
}
