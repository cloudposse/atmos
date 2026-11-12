// Package gitea integrates Atmos CI with Gitea Actions.
package gitea

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ProviderName identifies Gitea Actions.
const ProviderName = "gitea-actions"

const pullRequestTargetEvent = "pull_request_target"

// Provider implements Gitea Actions metadata and reporting.
type Provider struct{}

var _ provider.Provider = (*Provider)(nil)

var repositoryName = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// NewProvider creates a provider. Credentials are read only when making API requests.
func NewProvider() *Provider {
	defer perf.Track(nil, "gitea.NewProvider")()
	return &Provider{}
}
func init() { ci.Register(NewProvider()) }

// Name returns the registered provider name.
func (p *Provider) Name() string {
	defer perf.Track(nil, "gitea.Name")()
	return ProviderName
}

// Detect distinguishes Gitea from its GitHub-compatible environment.
func (p *Provider) Detect() bool {
	defer perf.Track(nil, "gitea.Detect")()
	return os.Getenv("GITEA_ACTIONS") == "true"
}

func env(name string) string {
	if value := os.Getenv("GITEA_" + name); value != "" {
		return value
	}
	return os.Getenv("GITHUB_" + name)
}

type revision struct {
	Ref  string `json:"ref"`
	SHA  string `json:"sha"`
	Repo *struct {
		FullName string `json:"full_name"`
	} `json:"repo"`
}
type event struct {
	Before      string `json:"before"`
	Number      int    `json:"number"`
	PullRequest *struct {
		Number int      `json:"number"`
		URL    string   `json:"html_url"`
		Head   revision `json:"head"`
		Base   revision `json:"base"`
	} `json:"pull_request"`
}

func readEvent() (*event, error) {
	path := env("EVENT_PATH")
	if path == "" {
		return nil, fmt.Errorf("%w: Gitea event payload path is missing", errUtils.ErrCIPullRequestUnknown)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Gitea event: %w", err)
	}
	var e event
	if err = json.Unmarshal(content, &e); err != nil {
		return nil, fmt.Errorf("parse Gitea event: %w", err)
	}
	return &e, nil
}

// Context returns runner metadata, using the actual checkout for commit status reporting.
func (p *Provider) Context() (*provider.Context, error) {
	defer perf.Track(nil, "gitea.Context")()

	number, _ := strconv.Atoi(env("RUN_NUMBER"))
	c := &provider.Context{
		Provider: ProviderName, RunID: env("RUN_ID"), RunNumber: number,
		Workflow: env("WORKFLOW"), Job: env("JOB"), Actor: env("ACTOR"), EventName: env("EVENT_NAME"),
		Ref: env("REF"), SHA: env("SHA"), Repository: env("REPOSITORY"), ServerURL: strings.TrimRight(env("SERVER_URL"), "/"),
	}
	if sha, err := git.NewDefaultGitRepo().GetCurrentCommitSHA(); err == nil && sha != "" {
		c.SHA = sha
	}
	if owner, repo, ok := strings.Cut(c.Repository, "/"); ok {
		c.RepoOwner, c.RepoName = owner, repo
	}
	c.Branch = env("HEAD_REF")
	if c.Branch == "" {
		c.Branch = env("REF_NAME")
	}
	setRepositoryURLs(c)
	c.ElevatedEvent = c.EventName == pullRequestTargetEvent || c.EventName == "workflow_run"
	if err := setPullRequestContext(c); err != nil {
		return nil, err
	}
	// An unsupported privileged event must not silently bypass fork checkout protection.
	if c.EventName == "workflow_run" {
		return nil, fmt.Errorf("%w: Gitea workflow_run context", errUtils.ErrCIOperationNotSupported)
	}
	return c, nil
}

// setPullRequestContext selects a fork head only for unprivileged PR events.
func setPullRequestContext(c *provider.Context) error {
	if c.EventName != "pull_request" && c.EventName != pullRequestTargetEvent {
		return nil
	}
	pr, err := pullRequestContext()
	if err != nil {
		return err
	}
	c.PullRequest = pr
	if c.EventName == pullRequestTargetEvent {
		return setBaseBranch(c, pr)
	}
	if !pr.Fork {
		return nil
	}
	return setForkContext(c, pr)
}

func setBaseBranch(c *provider.Context, pr *provider.PRInfo) error {
	c.Branch = pr.BaseRef
	if c.Branch == "" {
		c.Branch = env("BASE_REF")
	}
	if c.Branch == "" {
		return fmt.Errorf("%w: Gitea pull request base branch is missing", errUtils.ErrCIPullRequestUnknown)
	}
	return nil
}

func setForkContext(c *provider.Context, pr *provider.PRInfo) error {
	e, err := readEvent()
	if err != nil {
		return err
	}
	if e.PullRequest.Head.Repo == nil || !validRepositoryName(e.PullRequest.Head.Repo.FullName) || c.ServerURL == "" {
		return fmt.Errorf("%w: Gitea fork head repository is missing or invalid", errUtils.ErrCIPullRequestUnknown)
	}
	c.CloneURL = c.ServerURL + "/" + e.PullRequest.Head.Repo.FullName + ".git"
	if env("HEAD_REF") == "" && pr.HeadRef != "" {
		c.Branch = pr.HeadRef
	}
	return nil
}

// ResolveBase resolves PR head checkouts and push events. Unknown PR checkouts are
// deliberately left unresolved instead of comparing against an unsafe base.
func (p *Provider) ResolveBase() (*provider.BaseResolution, error) {
	defer perf.Track(nil, "gitea.ResolveBase")()

	name := env("EVENT_NAME")
	if name != "pull_request" && name != pullRequestTargetEvent && name != "push" {
		return nil, nil
	}
	e, err := readEvent()
	if err != nil {
		return nil, err
	}
	if name == "push" {
		return pushBase(e), nil
	}
	c, err := p.Context()
	if err != nil {
		return nil, err
	}
	if !isPRHead(e, c.SHA) {
		return nil, nil
	}
	return &provider.BaseResolution{
		SHA: e.PullRequest.Base.SHA, HeadSHA: e.PullRequest.Head.SHA,
		TargetBranch: e.PullRequest.Base.Ref, Source: "Gitea pull request base", EventType: name, Checkout: "head.sha",
	}, nil
}

// OutputWriter uses the runner's compatible environment files.
func (p *Provider) OutputWriter() provider.OutputWriter {
	defer perf.Track(nil, "gitea.OutputWriter")()

	return provider.NewFileOutputWriter(env("OUTPUT"), env("STEP_SUMMARY"))
}

func pullRequestContext() (*provider.PRInfo, error) {
	e, err := readEvent()
	if err != nil {
		return nil, err
	}
	pr := e.PullRequest
	if pr == nil {
		return nil, fmt.Errorf("%w: Gitea pull request payload is missing", errUtils.ErrCIPullRequestUnknown)
	}
	n := pr.Number
	if n == 0 {
		n = e.Number
	}
	if n <= 0 {
		return nil, fmt.Errorf("%w: Gitea pull request number is missing", errUtils.ErrCIPullRequestUnknown)
	}
	fork := pr.Head.Repo == nil || pr.Base.Repo == nil || pr.Head.Repo.FullName == "" || pr.Base.Repo.FullName == "" || pr.Head.Repo.FullName != pr.Base.Repo.FullName
	return &provider.PRInfo{Number: n, HeadRef: pr.Head.Ref, BaseRef: pr.Base.Ref, URL: pr.URL, Fork: fork}, nil
}

func validRepositoryName(name string) bool {
	if !repositoryName.MatchString(name) {
		return false
	}
	owner, repo, _ := strings.Cut(name, "/")
	return owner != "." && owner != ".." && repo != "." && repo != ".."
}

func setRepositoryURLs(c *provider.Context) {
	if c.ServerURL != "" && c.Repository != "" {
		c.CloneURL = c.ServerURL + "/" + c.Repository + ".git"
		if c.RunID != "" {
			c.RunURL = c.ServerURL + "/" + c.Repository + "/actions/runs/" + c.RunID
		}
	}
}

func pushBase(e *event) *provider.BaseResolution {
	if e.Before == "" || strings.Trim(e.Before, "0") == "" {
		return nil
	}
	return &provider.BaseResolution{SHA: e.Before, Source: "Gitea push event before", EventType: "push"}
}

func isPRHead(e *event, sha string) bool {
	return e.PullRequest != nil && e.PullRequest.Head.SHA != "" && sha == e.PullRequest.Head.SHA && e.PullRequest.Base.SHA != ""
}
