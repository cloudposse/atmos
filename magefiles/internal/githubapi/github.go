// Package githubapi contains the GitHub data shared by Mage reporting targets.
package githubapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
)

const (
	PageSize  = 100
	PageLimit = 100
)

type API interface {
	DoWithContext(context.Context, string, string, io.Reader, interface{}) error
	RequestWithContext(context.Context, string, string, io.Reader) (*http.Response, error)
}

type Run struct {
	ID             int64     `json:"id"`
	Attempt        int       `json:"run_attempt"`
	HeadSHA        string    `json:"head_sha"`
	Branch         string    `json:"head_branch"`
	Event          string    `json:"event"`
	Status         string    `json:"status"`
	Conclusion     string    `json:"conclusion"`
	Path           string    `json:"path"`
	Created        time.Time `json:"created_at"`
	Updated        time.Time `json:"updated_at"`
	HeadRepository struct {
		FullName string `json:"full_name"`
	} `json:"head_repository"`
	PullRequests []struct {
		Number int `json:"number"`
	} `json:"pull_requests"`
}

type PR struct {
	Updated      time.Time `json:"updated_at"`
	MergeSHA     string    `json:"merge_commit_sha"`
	ChangedFiles *int      `json:"changed_files"`
	Draft        bool      `json:"draft"`
	Number       int       `json:"number"`
	State        string    `json:"state"`
	// Mergeable is nil until GitHub has lazily computed it after a push or base move.
	Mergeable      *bool  `json:"mergeable"`
	MergeableState string `json:"mergeable_state"`
	Labels         []struct {
		Name string `json:"name"`
	} `json:"labels"`
	User struct {
		Login string `json:"login"`
		Type  string `json:"type"`
	} `json:"user"`
	Head struct {
		SHA  string `json:"sha"`
		Repo struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"head"`
	Base struct {
		Ref  string `json:"ref"`
		SHA  string `json:"sha"`
		Repo struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"base"`
}

type Comment struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
	User struct {
		Login string `json:"login"`
		Type  string `json:"type"`
	} `json:"user"`
}

func Get(ctx context.Context, client API, path string, response any) error {
	return client.DoWithContext(ctx, http.MethodGet, path, nil, response)
}

// ErrPRPaginationLimit reports that a PR listing exceeded PageLimit pages.
var ErrPRPaginationLimit = errors.New("PR pagination limit exceeded")

// ListOpenMainPRs lists open PRs against main from a paginated pulls endpoint.
// The path is a complete API path that already carries a query string. When
// headSHA is set, only PRs whose head is exactly that commit qualify, so a PR
// whose head moved after an event is never selected for the stale commit.
func ListOpenMainPRs(ctx context.Context, client API, path, headSHA string) ([]PR, error) {
	var matched []PR
	for page := 1; page <= PageLimit; page++ {
		var prs []PR
		if err := Get(ctx, client, fmt.Sprintf("%s&per_page=%d&page=%d", path, PageSize, page), &prs); err != nil {
			return nil, err
		}
		for i := range prs {
			pr := &prs[i]
			if pr.State == "open" && pr.Base.Ref == "main" && (headSHA == "" || pr.Head.SHA == headSHA) {
				matched = append(matched, *pr)
			}
		}
		if len(prs) < PageSize {
			return matched, nil
		}
	}
	return nil, ErrPRPaginationLimit
}

// OpenPRsForHead resolves open main PRs by their head branch. GitHub's commit
// association endpoint lists nothing for a commit that exists only in a fork,
// but the exact owner:branch lookup is strongly consistent. Every result's head
// is verified against sha.
func OpenPRsForHead(ctx context.Context, client API, repo string, head HeadRef) ([]PR, error) {
	query := url.QueryEscape(head.Owner + ":" + head.Branch)
	return ListOpenMainPRs(ctx, client, fmt.Sprintf("repos/%s/pulls?state=open&base=main&head=%s", repo, query), head.SHA)
}

// HeadRef identifies a PR head by its fork owner, branch and expected commit.
type HeadRef struct {
	Owner, Branch, SHA string
}

func NotFound(err error) bool {
	var httpErr *api.HTTPError
	return errors.As(err, &httpErr) && (httpErr.StatusCode == http.StatusNotFound || httpErr.StatusCode == http.StatusGone)
}

var (
	RepoPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	SHAPattern  = regexp.MustCompile(`^[0-9a-f]{40}$`)
)
var CommentRunPattern = regexp.MustCompile(`<!-- atmos-benchmark-run:(\d+):(\d+) -->`)
