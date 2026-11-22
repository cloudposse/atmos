package gitea

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// apiUnavailableMessage is logged when no usable Gitea API URL is configured.
const apiUnavailableMessage = "Gitea commit statuses and comments are disabled"

const (
	apiTimeout       = 30 * time.Second
	maxResponseBytes = 4 << 20
	pageSize         = 50
)

// apiURL returns the Gitea API base URL that requests may be sent to. A missing URL, or an
// HTTP URL without ATMOS_CI_GITEA_ALLOW_INSECURE=true, returns ErrGiteaAPIUnavailable: the
// run continues and only the API-backed features (statuses, comments) are skipped. A URL that
// is malformed, embeds credentials, or carries a query or fragment returns ErrInvalidURL.
func apiURL() (*url.URL, error) {
	raw := configuredAPIURL()
	if raw == "" {
		return nil, fmt.Errorf("%w: no Gitea API URL is configured (set ATMOS_CI_GITEA_API_URL or the runner's server URL)", errUtils.ErrGiteaAPIUnavailable)
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		(u.Scheme != "https" && u.Scheme != "http") {
		return nil, fmt.Errorf("%w: Gitea requires an absolute HTTPS API URL without credentials, query, or fragment", errUtils.ErrInvalidURL)
	}
	if !allowedScheme(u.Scheme) {
		return nil, fmt.Errorf("%w: Gitea requires an HTTPS API URL (HTTP requires ATMOS_CI_GITEA_ALLOW_INSECURE=true)", errUtils.ErrGiteaAPIUnavailable)
	}
	return u, nil
}

// ValidateConfig validates the endpoint before CI writes begin. Only a malformed URL stops the
// run. A missing or plain-HTTP URL disables the API-backed features with one log line, so the
// job summary and step outputs are still written through the runner's files.
func (p *Provider) ValidateConfig() error {
	defer perf.Track(nil, "gitea.ValidateConfig")()

	_, err := apiURL()
	if errors.Is(err, errUtils.ErrGiteaAPIUnavailable) {
		p.reportAPIUnavailable(err)
		return nil
	}
	return err
}

// reportAPIUnavailable logs once per provider why statuses and comments are skipped. It is a
// warning only when a token is configured, because then the user evidently expects API reporting.
func (p *Provider) reportAPIUnavailable(err error) {
	p.unavailableOnce.Do(func() {
		if _, tokenErr := apiToken(); tokenErr == nil {
			log.Warn(apiUnavailableMessage, "reason", err.Error())
			return
		}
		log.Debug(apiUnavailableMessage, "reason", err.Error())
	})
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
