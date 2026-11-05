//go:build mage

package release

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
)

func TestPrepareStopsWhenTagReservationFails(t *testing.T) {
	for _, mode := range []string{releaseModeCandidate, releaseModeCut} {
		t.Run(mode, func(t *testing.T) {
			env := releaseEnvironment()
			env["RELEASE_MODE"] = mode
			routes := planRoutes()
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method, "a failed reservation must not publish or overwrite a tag")
				if r.URL.Path == "/users/osterman" {
					githubtest.JSON(t, w, map[string]any{"id": 52489, "type": "User"})
					return
				}
				path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, strings.TrimSuffix(planPrefix, "/")), "/")
				if path == "tags" || strings.HasPrefix(path, "git/ref/tags/") {
					http.Error(w, "tag reservation unavailable", http.StatusServiceUnavailable)
					return
				}
				value, ok := routes[path]
				require.True(t, ok, "unexpected %s", r.URL)
				githubtest.JSON(t, w, value)
			})
			prepared, err := c.prepare(environmentReader(env))
			var responseError *api.HTTPError
			require.ErrorAs(t, err, &responseError)
			require.Equal(t, http.StatusServiceUnavailable, responseError.StatusCode)
			require.Nil(t, prepared, "failed preparation must not produce a build identity")
		})
	}
}

func TestCandidateNumberOverflowPreventsReservation(t *testing.T) {
	c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, planPrefix+"tags", r.URL.Path, "an invalid RC number must stop reservation before reading or writing Git objects")
		githubtest.JSON(t, w, []map[string]string{{"name": "v1.234.0-rc.18446744073709551616"}})
	})
	tag, err := c.reserveCandidate(preparedFixture())
	require.ErrorIs(t, err, ErrMilestonePolicy)
	require.ErrorContains(t, err, "RC number out of range")
	require.Empty(t, tag)
}

func TestCandidateReservationFailureBlocksPublicationStages(t *testing.T) {
	for _, stage := range []string{"draft", "validate", "summary"} {
		for _, failure := range []string{"annotation unavailable", "different owner"} {
			t.Run(stage+"/"+failure, func(t *testing.T) {
				p := preparedFixture()
				p.Mode, p.Tag = releaseModeCandidate, "v1.234.0-rc.1"
				candidate := newCandidateAPI(t)
				other := *p
				other.RunID = "88"
				candidate.add(&other)
				routes := planRoutes()
				reads := 0
				c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
					require.Equal(t, http.MethodGet, r.Method, "failed ownership checks must never publish")
					if strings.HasPrefix(r.URL.Path, planPrefix+"git/tags/") {
						reads++
						if failure == "annotation unavailable" {
							http.Error(w, "annotation unavailable", http.StatusServiceUnavailable)
							return
						}
					}
					if candidate.serve(w, r) {
						return
					}
					require.Equal(t, "validate", stage, "draft and summary must stop before querying releases")
					if r.URL.Path == "/users/osterman" {
						githubtest.JSON(t, w, map[string]any{"id": 52489, "type": "User"})
						return
					}
					path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, strings.TrimSuffix(planPrefix, "/")), "/")
					value, ok := routes[path]
					require.True(t, ok, "unexpected %s", r.URL)
					githubtest.JSON(t, w, value)
				})
				var err error
				switch stage {
				case "draft":
					var id int64
					id, err = c.draft(p)
					require.Zero(t, id)
				case "validate":
					env := releaseEnvironment()
					env["RELEASE_MODE"] = releaseModeCandidate
					err = c.validateLivePlan(p, environmentReader(env))
				case "summary":
					var output bytes.Buffer
					err = c.summarizeRelease(&p.Plan, 77, &output)
					require.Empty(t, output.String(), "failed verification must not claim publication")
					require.ErrorContains(t, err, "https://github.com/cloudposse/atmos/actions/runs/77")
				}
				require.Equal(t, 1, reads, "must verify the reserved tag annotation")
				if failure == "annotation unavailable" {
					var responseError *api.HTTPError
					require.ErrorAs(t, err, &responseError)
					require.Equal(t, http.StatusServiceUnavailable, responseError.StatusCode)
				} else {
					require.ErrorIs(t, err, ErrReleaseVerification)
				}
			})
		}
	}
}
