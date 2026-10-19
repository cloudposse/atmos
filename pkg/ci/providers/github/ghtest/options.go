package ghtest

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Option configures a Server.
type Option func(*Server)

// failure is an injected API error.
type failure struct {
	method     string
	pathPrefix string
	status     int
	message    string
}

// WithSeedComments pre-populates the comments of issue or pull request n so that
// an upsert can find an existing marker. Seeded comments are not recorded as writes.
func WithSeedComments(owner, repo string, n int, comments ...Comment) Option {
	defer perf.Track(nil, "ghtest.WithSeedComments")()

	return func(s *Server) {
		for i := range comments {
			c := comments[i]
			c.Owner, c.Repo, c.Number = owner, repo, n
			s.addComment(&c)
		}
	}
}

// WithSeedCommitComments pre-populates the comments of a commit so that an upsert can find an
// existing marker. Seeded comments are not recorded as writes.
func WithSeedCommitComments(owner, repo, sha string, comments ...Comment) Option {
	defer perf.Track(nil, "ghtest.WithSeedCommitComments")()

	return func(s *Server) {
		for i := range comments {
			c := comments[i]
			c.Owner, c.Repo, c.SHA, c.Number = owner, repo, sha, 0
			s.addComment(&c)
		}
	}
}

// WithFailure makes every request whose method matches and whose path starts
// with pathPrefix fail with the given HTTP status and a GitHub-style
// {"message": ...} body. An empty method or "*" matches any method. The first
// matching failure wins. Failed requests are still recorded by Requests.
func WithFailure(method, pathPrefix string, status int, message string) Option {
	defer perf.Track(nil, "ghtest.WithFailure")()

	return func(s *Server) {
		if method == "*" {
			method = ""
		}
		s.failures = append(s.failures, failure{
			method:     strings.ToUpper(method),
			pathPrefix: pathPrefix,
			status:     status,
			message:    message,
		})
	}
}

// WithPullRequests seeds the pull requests returned by the pulls list/get endpoints.
func WithPullRequests(owner, repo string, prs ...PullRequest) Option {
	defer perf.Track(nil, "ghtest.WithPullRequests")()

	return func(s *Server) {
		key := repoKey{owner, repo}
		s.pulls[key] = append(s.pulls[key], prs...)
	}
}

// WithCheckRuns seeds the check runs returned for a commit ref.
func WithCheckRuns(owner, repo, ref string, runs ...CheckRun) Option {
	defer perf.Track(nil, "ghtest.WithCheckRuns")()

	return func(s *Server) {
		key := refKey{owner, repo, ref}
		s.checkRuns[key] = append(s.checkRuns[key], runs...)
	}
}
