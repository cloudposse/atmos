//go:build mage

package release

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
)

// candidateAPI models immutable Git objects and atomic create-only references.
type candidateAPI struct {
	t            *testing.T
	refs         map[string]string
	objects      map[string]candidateTagObject
	writes       int
	lostResponse bool
	conflict     bool
}

func newCandidateAPI(t *testing.T) *candidateAPI {
	return &candidateAPI{t: t, refs: map[string]string{}, objects: map[string]candidateTagObject{}}
}

func (a *candidateAPI) add(p *preparedRelease) {
	message, err := json.Marshal(reservationFor(p))
	require.NoError(a.t, err)
	a.store(p.Tag, candidateMessagePrefix+string(message), p.Commit)
}

func (a *candidateAPI) store(tag, message, commit string) {
	sha := fmt.Sprintf("%040x", len(a.objects)+1)
	object := candidateTagObject{Tag: tag, Message: message}
	object.Object.Type, object.Object.SHA = "commit", commit
	a.objects[sha] = object
	a.refs[tag] = sha
}

func (a *candidateAPI) serve(w http.ResponseWriter, r *http.Request) bool {
	path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, strings.TrimSuffix(planPrefix, "/")), "/")
	switch {
	case path == "tags":
		require.Equal(a.t, http.MethodGet, r.Method)
		tags := []map[string]string{}
		for tag := range a.refs {
			tags = append(tags, map[string]string{"name": tag})
		}
		page, err := strconv.Atoi(r.URL.Query().Get("page"))
		require.NoError(a.t, err)
		// Tests requiring multiple pages use their own deterministic handler.
		start := min((page-1)*100, len(tags))
		githubtest.JSON(a.t, w, tags[start:min(start+100, len(tags))])
	case path == "git/tags" && r.Method == http.MethodPost:
		a.writes++
		var body map[string]string
		require.NoError(a.t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(a.t, "commit", body["type"])
		sha := fmt.Sprintf("%040x", len(a.objects)+1)
		object := candidateTagObject{Tag: body["tag"], Message: body["message"]}
		object.Object.Type, object.Object.SHA = "commit", body["object"]
		a.objects[sha] = object
		githubtest.JSON(a.t, w, map[string]string{"sha": sha})
	case path == "git/refs" && r.Method == http.MethodPost:
		a.writes++
		var body map[string]string
		require.NoError(a.t, json.NewDecoder(r.Body).Decode(&body))
		tag := strings.TrimPrefix(body["ref"], "refs/tags/")
		if a.conflict {
			other := preparedFixture()
			other.Tag, other.Mode, other.RunID = tag, releaseModeCandidate, "88"
			a.add(other)
		}
		if _, exists := a.refs[tag]; exists {
			http.Error(w, "reference already exists", http.StatusUnprocessableEntity)
			return true
		}
		a.refs[tag] = body["sha"]
		if a.lostResponse {
			http.Error(w, "response lost", http.StatusServiceUnavailable)
		} else {
			githubtest.JSON(a.t, w, map[string]any{})
		}
	case strings.HasPrefix(path, "git/ref/tags/"):
		tag := strings.TrimPrefix(path, "git/ref/tags/")
		sha, ok := a.refs[tag]
		if !ok {
			http.NotFound(w, r)
			return true
		}
		kind := "commit"
		if _, annotated := a.objects[sha]; annotated {
			kind = "tag"
		}
		githubtest.JSON(a.t, w, tagObject(kind, sha))
	case strings.HasPrefix(path, "git/tags/"):
		object, ok := a.objects[strings.TrimPrefix(path, "git/tags/")]
		if !ok {
			http.NotFound(w, r)
			return true
		}
		githubtest.JSON(a.t, w, object)
	default:
		return false
	}
	return true
}

func TestSequentialCandidatesAndRetry(t *testing.T) {
	a := newCandidateAPI(t)
	c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		require.True(t, a.serve(w, r), "unexpected %s", r.URL)
	})
	p := preparedFixture()
	p.Mode = releaseModeCandidate
	first, err := c.reserveCandidate(p)
	require.NoError(t, err)
	require.Equal(t, "v1.234.0-rc.1", first)
	require.Equal(t, 2, a.writes)
	p.Tag = first
	require.NoError(t, c.verifyCandidateReservation(p))
	retry, err := c.reserveCandidate(p)
	require.NoError(t, err)
	require.Equal(t, first, retry)
	require.Equal(t, 2, a.writes)
	p.RunID = "78"
	second, err := c.reserveCandidate(p)
	require.NoError(t, err)
	require.Equal(t, "v1.234.0-rc.2", second, "a failed build still consumes its reserved number")
	p.RunID = "77"
	retry, err = c.reserveCandidate(p)
	require.NoError(t, err)
	require.Equal(t, first, retry, "a retry after another run must retain the original number")
	p.Commit = githubtest.Base
	_, err = c.reserveCandidate(p)
	require.ErrorContains(t, err, "conflicting RC reservation")
	require.Equal(t, 4, a.writes)
}

func TestCandidateNumericOrderAndVersionIsolation(t *testing.T) {
	a := newCandidateAPI(t)
	for _, tag := range []string{"v1.234.0-rc.9", "v1.234.0-rc.10", "v1.234.0-rc.0", "v1.234.0-rc.099", "v1.234.0-rc.50.extra", "v1.235.0-rc.100", "v1.234.0-nightly.100"} {
		a.refs[tag] = githubtest.SHA
	}
	c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) { require.True(t, a.serve(w, r)) })
	p := preparedFixture()
	p.Mode = releaseModeCandidate
	tag, err := c.reserveCandidate(p)
	require.NoError(t, err)
	require.Equal(t, "v1.234.0-rc.11", tag)
	p.Version = "v1.233.2"
	tag, err = c.reserveCandidate(p)
	require.NoError(t, err)
	require.Equal(t, "v1.233.2-rc.1", tag)
}

func TestCandidateReservationConflictsAndLostResponse(t *testing.T) {
	for _, mode := range []string{"lost response", "conflict", "duplicate run", "tampered annotation", "overflow"} {
		t.Run(mode, func(t *testing.T) {
			a := newCandidateAPI(t)
			p := preparedFixture()
			p.Mode, p.Tag = releaseModeCandidate, "v1.234.0-rc.1"
			switch mode {
			case "lost response":
				a.lostResponse = true
			case "conflict":
				a.conflict = true
			case "duplicate run":
				a.add(p)
				other := *p
				other.Tag = "v1.234.0-rc.2"
				a.add(&other)
			case "tampered annotation":
				a.add(p)
				object := a.objects[a.refs[p.Tag]]
				object.Object.SHA = githubtest.Base
				a.objects[a.refs[p.Tag]] = object
			case "overflow":
				a.refs["v1.234.0-rc.18446744073709551615"] = githubtest.SHA
			}
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) { require.True(t, a.serve(w, r)) })
			tag, err := c.reserveCandidate(p)
			if mode == "lost response" {
				require.NoError(t, err)
				require.Equal(t, p.Tag, tag)
			} else {
				require.Error(t, err)
				require.Empty(t, tag)
			}
			if mode == "conflict" {
				r, err := c.readCandidateReservation(p.Tag)
				require.NoError(t, err)
				require.Equal(t, "88", r.RunID)
				require.Error(t, c.verifyCandidateReservation(p))
			}
		})
	}
}

func TestCandidatePagination(t *testing.T) {
	pages := []string{}
	c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, strings.TrimSuffix(planPrefix, "/")), "/")
		if path == "tags" {
			pages = append(pages, r.URL.Query().Get("page"))
			tags := []map[string]string{{"name": "v1.234.0-rc.12"}}
			if r.URL.Query().Get("page") == "1" {
				tags = make([]map[string]string, 100)
				for i := range tags {
					tags[i] = map[string]string{"name": fmt.Sprintf("v1.0.%d", i)}
				}
			}
			githubtest.JSON(t, w, tags)
			return
		}
		require.Equal(t, "git/ref/tags/v1.234.0-rc.12", path)
		githubtest.JSON(t, w, tagObject("commit", githubtest.SHA))
	})
	n, owned, err := c.candidateReservations(preparedFixture())
	require.NoError(t, err)
	require.EqualValues(t, 12, n)
	require.Empty(t, owned)
	require.Equal(t, []string{"1", "2"}, pages)
}

func TestCandidateAPIFailuresDoNotReserveAnotherTag(t *testing.T) {
	for _, failure := range []string{"list", "create object", "invalid object SHA", "create ref", "read ref", "read annotation"} {
		t.Run(failure, func(t *testing.T) {
			a := newCandidateAPI(t)
			p := preparedFixture()
			p.Mode, p.Tag = releaseModeCandidate, "v1.234.0-rc.1"
			if strings.HasPrefix(failure, "read") {
				a.add(p)
			}
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				path := strings.TrimPrefix(r.URL.Path, planPrefix)
				if failure == "list" && path == "tags" ||
					failure == "create object" && path == "git/tags" ||
					failure == "create ref" && path == "git/refs" ||
					failure == "read ref" && strings.HasPrefix(path, "git/ref/") ||
					failure == "read annotation" && strings.HasPrefix(path, "git/tags/") {
					http.Error(w, "credentials unavailable", http.StatusUnauthorized)
					return
				}
				if failure == "invalid object SHA" && path == "git/tags" {
					githubtest.JSON(t, w, map[string]string{"sha": "invalid"})
					return
				}
				require.True(t, a.serve(w, r))
			})
			tag, err := c.reserveCandidate(p)
			require.Error(t, err)
			require.Empty(t, tag)
			if strings.HasPrefix(failure, "read") {
				require.Len(t, a.refs, 1)
			} else {
				require.Empty(t, a.refs)
			}
		})
	}
}

func TestCandidateRejectsMalformedOrUnownedAnnotations(t *testing.T) {
	for _, failure := range []string{"JSON", "tag", "object type", "repository", "run ID", "version", "zero", "number overflow"} {
		t.Run(failure, func(t *testing.T) {
			p := preparedFixture()
			p.Mode, p.Tag = releaseModeCandidate, "v1.234.0-rc.1"
			r := reservationFor(p)
			switch failure {
			case "repository":
				r.Repository = "other/repo"
			case "run ID":
				r.RunID = "077"
			case "version":
				r.Version = "v1.235.0"
			case "zero":
				p.Tag = "v1.234.0-rc.0"
			case "number overflow":
				p.Tag = "v1.234.0-rc.18446744073709551616"
			}
			message, err := json.Marshal(r)
			require.NoError(t, err)
			object := candidateTagObject{Tag: p.Tag, Message: candidateMessagePrefix + string(message)}
			object.Object.Type, object.Object.SHA = "commit", p.Commit
			switch failure {
			case "JSON":
				object.Message = candidateMessagePrefix + "{"
			case "tag":
				object.Tag = "v1.234.0-rc.2"
			case "object type":
				object.Object.Type = "tag"
			}
			_, err = parseCandidateReservation(p.Repository, p.Tag, object)
			require.Error(t, err)
		})
	}
	a := newCandidateAPI(t)
	a.store("v1.234.0-rc.1", "Historical annotated tag", githubtest.SHA)
	c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) { require.True(t, a.serve(w, r)) })
	p := preparedFixture()
	p.Mode = releaseModeCandidate
	tag, err := c.reserveCandidate(p)
	require.NoError(t, err)
	require.Equal(t, "v1.234.0-rc.2", tag)
	for _, tag := range []string{"v1.234.0-rc.", "v1.234.0-rc.0", "v1.235.0-rc.1"} {
		p.Tag = tag
		require.Error(t, validatePrepared(p))
	}
}

func TestCandidateSummaryDoesNotAdoptAnotherRun(t *testing.T) {
	a := newCandidateAPI(t)
	p := preparedFixture()
	p.Mode, p.Tag = releaseModeCandidate, "v1.234.0-rc.1"
	a.add(p)
	c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.True(t, a.serve(w, r), "must not claim publication without this run's reservation")
	})
	var output bytes.Buffer
	err := c.summarizeRelease(&p.Plan, 88, &output)
	require.ErrorContains(t, err, "has no reserved RC")
	require.Empty(t, output.String())
}
