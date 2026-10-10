// Package githubapi contains the GitHub data shared by Mage reporting targets.
package githubapi

import (
	"context"
	"errors"
	"io"
	"net/http"
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
	MergeSHA     string `json:"merge_commit_sha"`
	ChangedFiles *int   `json:"changed_files"`
	Draft        bool   `json:"draft"`
	Number       int    `json:"number"`
	State        string `json:"state"`
	Head         struct {
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

func NotFound(err error) bool {
	var httpErr *api.HTTPError
	return errors.As(err, &httpErr) && (httpErr.StatusCode == http.StatusNotFound || httpErr.StatusCode == http.StatusGone)
}

var (
	RepoPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	SHAPattern  = regexp.MustCompile(`^[0-9a-f]{40}$`)
)
var CommentRunPattern = regexp.MustCompile(`<!-- atmos-benchmark-run:(\d+):(\d+) -->`)
