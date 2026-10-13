package ghtest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/cloudposse/atmos/pkg/perf"
)

type (
	repoKey  struct{ owner, repo string }
	issueKey struct {
		owner, repo string
		number      int
	}
	refKey struct{ owner, repo, ref string }
)

// commitKey identifies the commit comments of one commit.
type commitKey struct{ owner, repo, sha string }

// firstCommentID is the first ID assigned to a created comment.
const firstCommentID = 1000

// Server is a fake GitHub REST API backed by httptest.Server that records every
// write the CI provider makes. It is test support; never use it in production code.
//
// Unimplemented routes answer 404 with a JSON message, so unexpected calls are
// visible both as errors in the code under test and in Requests.
type Server struct {
	srv *httptest.Server

	mu       sync.Mutex
	failures []failure

	// comments is the current state of each issue's comments, in creation order.
	comments map[issueKey][]Comment
	// commitComments is the current state of each commit's comments, in creation order.
	commitComments map[commitKey][]Comment
	// commentWrites is the log of created and edited comments (issue and commit), in order.
	commentWrites []Comment
	nextCommentID int64

	statuses []Status

	sarif     []SARIFUpload
	nextSARIF int

	checkRuns map[refKey][]CheckRun
	pulls     map[repoKey][]PullRequest

	requests []RecordedRequest

	// afterRoute, when set, runs after a route handler returns and before the
	// request's status is recorded. Internal tests use it to hold a handler open
	// and prove that Requests already lists the request.
	afterRoute func()
}

// NewServer starts a fake GitHub API and registers its shutdown with t.Cleanup.
func NewServer(t testing.TB, opts ...Option) *Server {
	defer perf.Track(nil, "ghtest.NewServer")()

	t.Helper()

	s := &Server{
		comments:       map[issueKey][]Comment{},
		commitComments: map[commitKey][]Comment{},
		nextCommentID:  firstCommentID,
		checkRuns:      map[refKey][]CheckRun{},
		pulls:          map[repoKey][]PullRequest{},
	}
	for _, opt := range opts {
		opt(s)
	}

	s.srv = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.srv.Close)

	return s
}

// URL returns the server base URL, suitable for GITHUB_API_URL.
func (s *Server) URL() string {
	defer perf.Track(nil, "ghtest.Server.URL")()

	return s.srv.URL
}

// Comments returns every comment write (create and edit) in the order received.
// Edits carry the submitted body and Edited=true.
func (s *Server) Comments() []Comment {
	defer perf.Track(nil, "ghtest.Server.Comments")()

	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]Comment(nil), s.commentWrites...)
}

// CommentsFor returns the current comments of an issue or pull request,
// including seeded ones, with edits applied.
func (s *Server) CommentsFor(owner, repo string, n int) []Comment {
	defer perf.Track(nil, "ghtest.Server.CommentsFor")()

	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]Comment(nil), s.comments[issueKey{owner, repo, n}]...)
}

// CommitCommentsFor returns the current comments of a commit, including seeded ones, with edits applied.
func (s *Server) CommitCommentsFor(owner, repo, sha string) []Comment {
	defer perf.Track(nil, "ghtest.Server.CommitCommentsFor")()

	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]Comment(nil), s.commitComments[commitKey{owner, repo, sha}]...)
}

// Statuses returns every commit status written, in the order received.
func (s *Server) Statuses() []Status {
	defer perf.Track(nil, "ghtest.Server.Statuses")()

	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]Status(nil), s.statuses...)
}

// SARIFUploads returns every code-scanning upload received, decoded.
func (s *Server) SARIFUploads() []SARIFUpload {
	defer perf.Track(nil, "ghtest.Server.SARIFUploads")()

	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]SARIFUpload(nil), s.sarif...)
}

// Requests returns every HTTP request received (including failed and unexpected
// ones), in order. Intended for debugging and for asserting "no unexpected calls".
func (s *Server) Requests() []RecordedRequest {
	defer perf.Track(nil, "ghtest.Server.Requests")()

	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]RecordedRequest(nil), s.requests...)
}

// addComment stores a comment (assigning an ID when zero) into the current state.
// Callers must hold s.mu or be single-threaded (option application).
func (s *Server) addComment(c *Comment) Comment {
	if c.ID == 0 {
		c.ID = s.nextCommentID
	}
	if c.ID >= s.nextCommentID {
		s.nextCommentID = c.ID + 1
	}
	if c.HTMLURL == "" {
		c.HTMLURL = commentURL(c)
	}
	if c.SHA != "" {
		key := commitKey{c.Owner, c.Repo, c.SHA}
		s.commitComments[key] = append(s.commitComments[key], *c)
		return *c
	}
	key := issueKey{c.Owner, c.Repo, c.Number}
	s.comments[key] = append(s.comments[key], *c)

	return *c
}

// statusRecorder captures the status code written by a handler.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	defer perf.Track(nil, "ghtest.statusRecorder.WriteHeader")()

	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// serve records the request on arrival, applies injected failures, dispatches to the
// routes, and fills in the status once the handler returns.
//
// Recording on arrival matters: a response body larger than net/http's write buffer
// reaches the client before the handler returns, and go-github finishes decoding a
// page as soon as the JSON value is complete. The client can therefore issue the next
// request, and a test can read Requests, while this handler is still unwinding. If
// the record were appended afterwards, Requests would be missing or reorder the
// request that was served first.
func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body))

	s.mu.Lock()
	index := len(s.requests)
	s.requests = append(s.requests, RecordedRequest{
		Method:   r.Method,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
		Body:     string(body),
	})
	s.mu.Unlock()

	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.requests[index].Status = rec.status
	}()

	if f, ok := s.matchFailure(r); ok {
		writeError(rec, f.status, f.message)
		return
	}

	s.routes().ServeHTTP(rec, r)
	if s.afterRoute != nil {
		s.afterRoute()
	}
}

func (s *Server) matchFailure(r *http.Request) (failure, bool) {
	for _, f := range s.failures {
		if (f.method == "" || f.method == r.Method) && strings.HasPrefix(r.URL.Path, f.pathPrefix) {
			return f, true
		}
	}

	return failure{}, false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"message": message})
}
