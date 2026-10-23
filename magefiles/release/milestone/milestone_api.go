package milestone

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/magefile/mage/mg"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
)

// Milestones groups milestone lifecycle and merge-policy Mage targets.
type Milestones mg.Namespace

type Client struct {
	Context context.Context
	API     githubapi.API
	Repo    string
}

func NewClient(ctx context.Context, repo string) (*Client, error) {
	if !githubapi.RepoPattern.MatchString(repo) {
		return nil, fmt.Errorf("%w: invalid repository", ErrMilestonePolicy)
	}
	client, err := api.DefaultRESTClient()
	if err != nil {
		return nil, err
	}
	return &Client{Context: ctx, API: client, Repo: repo}, nil
}

func (c *Client) Get(path string, out any) error {
	return githubapi.Get(c.Context, c.API, "repos/"+c.Repo+"/"+path, out)
}

func (c *Client) Write(method, path string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return c.API.DoWithContext(c.Context, method, "repos/"+c.Repo+"/"+path, bytes.NewReader(data), nil)
}

func Pages[T any](c *Client, path string) ([]T, error) {
	var all []T
	for page := 1; page <= githubapi.PageLimit; page++ {
		var items []T
		err := c.Get(fmt.Sprintf("%s&per_page=100&page=%d", path, page), &items)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		if len(items) < githubapi.PageSize {
			return all, nil
		}
	}
	return nil, fmt.Errorf("%w: pagination limit for %s", ErrMilestonePolicy, path)
}

func (c *Client) ListMilestones() ([]Milestone, error) {
	return Pages[Milestone](c, "milestones?state=all")
}

func (c *Client) LatestStable() (string, error) {
	releases, err := Pages[struct {
		Tag        string     `json:"tag_name"`
		Draft      bool       `json:"draft"`
		Prerelease bool       `json:"prerelease"`
		Published  *time.Time `json:"published_at"`
	}](c, "releases?")
	if err != nil {
		return "", err
	}
	latest := ""
	for _, r := range releases {
		if r.Draft || r.Prerelease || r.Published == nil {
			continue
		}
		v, parseErr := MilestoneVersion(r.Tag)
		if parseErr != nil {
			continue
		}
		if latest == "" {
			latest = r.Tag
			continue
		}
		old, _ := MilestoneVersion(latest)
		if v.GreaterThan(old) {
			latest = r.Tag
		}
	}
	if latest == "" {
		return "", fmt.Errorf("%w: no published stable baseline", ErrMilestonePolicy)
	}
	return latest, nil
}

// CreateMilestone creates a manually chosen patch, minor or major milestone.
// Repeating the operation reuses the exact semantic version; labels never pick it.
func (Milestones) Create(repo, version string) error {
	if _, err := MilestoneVersion(version); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, err := NewClient(ctx, repo)
	if err != nil {
		return err
	}
	return client.ensureMilestone(version)
}

func (c *Client) ensureMilestone(title string) error {
	desired, err := MilestoneVersion(title)
	if err != nil {
		return err
	}
	milestones, err := c.ListMilestones()
	if err != nil {
		return err
	}
	matches := matchingMilestones(desired.String(), milestones)
	if len(matches) > 1 {
		return fmt.Errorf("%w: duplicate milestones for %s", ErrMilestonePolicy, title)
	}
	if len(matches) == 1 {
		if matches[0].State == "open" {
			return nil
		}
		return fmt.Errorf("%w: milestone %s already exists and is closed", ErrMilestonePolicy, title)
	}
	err = c.Write(http.MethodPost, "milestones", map[string]string{"title": title, "state": "open"})
	if err == nil {
		return nil
	}
	return c.recoverMilestoneCreation(desired.String(), err)
}

func matchingMilestones(version string, milestones []Milestone) []Milestone {
	var matches []Milestone
	for _, m := range milestones {
		v, err := MilestoneVersion(m.Title)
		if err == nil && v.String() == version {
			matches = append(matches, m)
		}
	}
	return matches
}

// Creation can succeed even when its response is lost. Re-read on failure;
// accepting a single matching open milestone makes retries idempotent.
func (c *Client) recoverMilestoneCreation(version string, writeErr error) error {
	fresh, readErr := c.ListMilestones()
	if readErr != nil {
		return errors.Join(writeErr, readErr)
	}
	count := 0
	for _, m := range matchingMilestones(version, fresh) {
		if m.State == "open" {
			count++
		}
	}
	if count == 1 {
		return nil
	}
	return writeErr
}

// EnsureMinorMilestones keeps the next three minor milestones open. Patch and
// major milestones are created only through the explicit CreateMilestone target.
func (Milestones) Ensure(repo string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, err := NewClient(ctx, repo)
	if err != nil {
		return err
	}
	return client.ensureMinorMilestones()
}

func (c *Client) ensureMinorMilestones() error {
	baseline, err := c.LatestStable()
	if err != nil {
		return err
	}
	milestones, err := c.ListMilestones()
	if err != nil {
		return err
	}
	changes, err := MinorMilestoneWindow(baseline, milestones)
	if err != nil {
		return err
	}
	for _, change := range changes {
		if change.Number == 0 {
			err = c.ensureMilestone(change.Title)
		} else {
			// Do not reopen a milestone if a release advanced while we were planning.
			current, readErr := c.LatestStable()
			if readErr != nil {
				return readErr
			}
			if current != baseline {
				return fmt.Errorf("%w: stable release changed; retry reconciliation", ErrMilestonePolicy)
			}
			err = c.Write(http.MethodPatch, fmt.Sprintf("milestones/%d", change.Number), map[string]string{"state": "open"})
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) OpenPRs(base string) ([]ReleasePR, error) {
	return Pages[ReleasePR](c, "pulls?state=open&base="+url.QueryEscape(base))
}

// Run is the action adapter. Invalid operations fail before any API request;
// workflow YAML only transports arguments, never implements dispatch policy.
func (Milestones) Run(repo, operation, version string) error {
	switch operation {
	case "check":
		return (Milestones{}).Check(repo)
	case "ensure":
		return (Milestones{}).Ensure(repo)
	case "create":
		return (Milestones{}).Create(repo, version)
	default:
		return fmt.Errorf("%w: unknown milestone operation %q", ErrMilestonePolicy, operation)
	}
}
