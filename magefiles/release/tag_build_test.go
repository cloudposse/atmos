//go:build mage

package release

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/stretchr/testify/require"
)

func taggedPlan() *preparedRelease {
	return &preparedRelease{Plan: Plan{Repository: githubtest.Repo, Mode: releaseModeCut, Version: "v1.235.0", Commit: githubtest.SHA, Branch: "main"}, RunID: "77", Tag: "v1.235.0"}
}

func taggedRun(p *preparedRelease) buildRun {
	r := buildRun{ID: 99, Attempt: 1, Title: tagBuildTitle(p, "88"), Path: taggedBuildWorkflow, HeadSHA: p.Commit, HeadBranch: p.Tag, Event: "workflow_dispatch", Status: "completed", Conclusion: "success"}
	r.Repository.FullName = p.Repository
	return r
}

func TestTagSigningVersionBoundary(t *testing.T) {
	for _, version := range []string{"v1.227.0", "v1.233.0", "v1.233.2", "v1.234.0", "v1.235.0", "v1.236.0", "invalid"} {
		for _, mode := range []string{releaseModeCut, releaseModeCandidate, releaseModeNightly, releaseModeFeature} {
			p := taggedPlan()
			p.Version = version
			p.Mode = mode
			if mode == releaseModeCandidate {
				p.Tag = version + "-rc.1"
			}
			want := (version == "v1.235.0" || version == "v1.236.0") && (mode == releaseModeCut || mode == releaseModeCandidate)
			require.Equal(t, want, taggedSigning(p), "%s %s", mode, version)
		}
	}
}

func TestTagBuildDispatchAndRetryRecovery(t *testing.T) {
	for _, mode := range []string{"dispatch", "reuse", "failed child", "recovered child", "duplicate", "missing on retry", "missing after dispatch", "wrong tag", "wrong SHA", "wrong workflow", "wrong repository", "dispatch failure"} {
		t.Run(mode, func(t *testing.T) {
			p := taggedPlan()
			run := taggedRun(p)
			writes, polls := 0, 0
			attempt := "1"
			if mode == "missing on retry" || mode == "recovered child" {
				attempt = "2"
			}
			if mode == "recovered child" {
				run.Attempt = 2
			}
			if mode == "failed child" {
				run.Conclusion = "failure"
			}
			if mode == "wrong tag" {
				run.HeadBranch = "main"
			}
			if mode == "wrong SHA" {
				run.HeadSHA = strings.Repeat("b", 40)
			}
			if mode == "wrong workflow" {
				run.Path = ".github/workflows/other.yml"
			}
			if mode == "wrong repository" {
				run.Repository.FullName = "other/atmos"
			}
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					writes++
					require.Contains(t, r.URL.Path, "/release-tag-build.yml/dispatches")
					var body struct {
						Ref    string            `json:"ref"`
						Inputs map[string]string `json:"inputs"`
					}
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					require.Equal(t, p.Tag, body.Ref)
					require.Equal(t, p.RunID, body.Inputs["parent-run"])
					require.Equal(t, "88", body.Inputs["plan-artifact"])
					require.Equal(t, tagBuildRequest(p, "88"), body.Inputs["request-id"])
					if mode == "dispatch failure" {
						http.Error(w, "unavailable", http.StatusServiceUnavailable)
					} else {
						w.WriteHeader(http.StatusNoContent)
					}
					return
				}
				polls++
				require.Contains(t, r.URL.Path, "/release-tag-build.yml/runs")
				runs := []buildRun{run}
				if mode == "missing on retry" || mode == "missing after dispatch" || mode == "dispatch failure" || (mode == "dispatch" && writes == 0) {
					runs = nil
				}
				if mode == "duplicate" {
					other := run
					other.ID++
					runs = append(runs, other)
				}
				githubtest.JSON(t, w, map[string]any{"workflow_runs": runs})
			})
			got, err := c.followTagBuild(p, "88", attempt, func(context.Context) error { return context.DeadlineExceeded })
			if mode == "dispatch" || mode == "reuse" || mode == "recovered child" {
				require.NoError(t, err)
				require.Equal(t, run.ID, got.ID)
				require.Equal(t, run.Attempt, got.Attempt)
			} else {
				require.Error(t, err)
			}
			expected := 0
			if mode == "dispatch" || mode == "missing after dispatch" || mode == "dispatch failure" {
				expected = 1
			}
			require.Equal(t, expected, writes)
			require.Positive(t, polls)
		})
	}
}

func TestTagBuildDetectsDuplicateOnLaterPage(t *testing.T) {
	p := taggedPlan()
	run := taggedRun(p)
	c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		runs := []buildRun{run}
		if r.URL.Query().Get("page") == "1" {
			for len(runs) < 100 {
				runs = append(runs, buildRun{ID: int64(len(runs) + 100), Title: "another request"})
			}
		} else {
			runs[0].ID++
		}
		githubtest.JSON(t, w, map[string]any{"workflow_runs": runs})
	})
	_, err := c.findTagBuild(p, "88")
	require.ErrorContains(t, err, "duplicate")
}

func TestTagBuildRequiresOriginalParentArtifact(t *testing.T) {
	for _, mode := range []string{"valid", "invalid ID", "wrong parent", "wrong workflow", "wrong branch", "wrong actor", "wrong rerunner", "expired", "wrong artifact run", "wrong artifact SHA", "wrong name"} {
		t.Run(mode, func(t *testing.T) {
			p := taggedPlan()
			parent := taggedRun(p)
			parent.ID = 77
			parent.Path = ".github/workflows/publish-release.yml"
			parent.HeadBranch = "main"
			parent.Actor = buildActor{ID: 52489, Login: "osterman"}
			parent.TriggeringActor = parent.Actor
			a := buildArtifact{ID: 88, Name: "release-plan-77-1"}
			a.WorkflowRun.ID = 77
			a.WorkflowRun.HeadSHA = parent.HeadSHA
			id := "88"
			switch mode {
			case "invalid ID":
				id = "088"
			case "wrong parent":
				parent.ID++
			case "wrong workflow":
				parent.Path = taggedBuildWorkflow
			case "wrong branch":
				parent.HeadBranch = "feature"
			case "wrong actor":
				parent.Actor.ID = 123
			case "wrong rerunner":
				parent.TriggeringActor.ID = 123
			case "expired":
				a.Expired = true
			case "wrong artifact run":
				a.WorkflowRun.ID++
			case "wrong artifact SHA":
				a.WorkflowRun.HeadSHA = strings.Repeat("b", 40)
			case "wrong name":
				a.Name = "release-plan-99-1"
			}
			calls := 0
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if strings.Contains(r.URL.Path, "/artifacts/") {
					githubtest.JSON(t, w, a)
				} else {
					githubtest.JSON(t, w, parent)
				}
			})
			_, err := c.checkBuildPlanArtifact(p, id)
			if mode == "valid" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrReleaseVerification)
			}
			if mode == "invalid ID" {
				require.Zero(t, calls)
			}
		})
	}
}

func TestTagBuildCollectsOnlySuccessfulAttemptArtifact(t *testing.T) {
	for _, mode := range []string{"valid", "missing", "duplicate", "expired", "wrong run", "wrong SHA", "old attempt"} {
		t.Run(mode, func(t *testing.T) {
			run := taggedRun(taggedPlan())
			run.Attempt = 2
			a := buildArtifact{ID: 100, Name: "release-build-99-2"}
			a.WorkflowRun.ID = run.ID
			a.WorkflowRun.HeadSHA = run.HeadSHA
			switch mode {
			case "expired":
				a.Expired = true
			case "wrong run":
				a.WorkflowRun.ID++
			case "wrong SHA":
				a.WorkflowRun.HeadSHA = "other"
			case "old attempt":
				a.Name = "release-build-99-1"
			}
			artifacts := []buildArtifact{a}
			if mode == "missing" {
				artifacts = nil
			}
			if mode == "duplicate" {
				artifacts = append(artifacts, a)
			}
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				require.Contains(t, r.URL.Path, "/runs/99/artifacts")
				githubtest.JSON(t, w, map[string]any{"artifacts": artifacts})
			})
			got, err := c.tagBuildArtifact(&run)
			if mode == "valid" {
				require.NoError(t, err)
				require.Equal(t, a.ID, got.ID)
			} else {
				require.ErrorIs(t, err, ErrReleaseVerification)
			}
		})
	}
}

func TestTagBuildRejectsWrongDispatchIdentityBeforeAPI(t *testing.T) {
	for _, mode := range []string{"ref", "SHA", "workflow SHA", "workflow ref", "event", "parent", "request", "artifact", "old version", "nightly"} {
		t.Run(mode, func(t *testing.T) {
			p := taggedPlan()
			artifact, parent, request := "88", p.RunID, tagBuildRequest(p, "88")
			env := map[string]string{"GITHUB_EVENT_NAME": "workflow_dispatch", "GITHUB_REF": "refs/tags/" + p.Tag, "GITHUB_SHA": p.Commit, "GITHUB_WORKFLOW_SHA": p.Commit, "GITHUB_WORKFLOW_REF": p.Repository + "/" + taggedBuildWorkflow + "@refs/tags/" + p.Tag}
			switch mode {
			case "ref":
				env["GITHUB_REF"] = "refs/heads/main"
			case "SHA":
				env["GITHUB_SHA"] = "other"
			case "workflow SHA":
				env["GITHUB_WORKFLOW_SHA"] = "other"
			case "workflow ref":
				env["GITHUB_WORKFLOW_REF"] = "other"
			case "event":
				env["GITHUB_EVENT_NAME"] = "push"
			case "parent":
				parent = "78"
			case "request":
				request = "wrong"
			case "artifact":
				artifact = "89"
			case "old version":
				p.Version = "v1.234.0"
			case "nightly":
				p.Mode = releaseModeNightly
			}
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				t.Error("invalid identity must fail before GitHub calls")
				http.Error(w, "unexpected", http.StatusInternalServerError)
			})
			require.ErrorIs(t, c.validateTagBuild(p, artifact, parent, request, environmentReader(env)), ErrReleaseVerification)
		})
	}
}

func TestTaggedChildRevalidatesParentPlanAndReservation(t *testing.T) {
	for _, mode := range []string{"cut", "candidate", "moved tag", "unauthorized rerun", "changed milestone", "lost reservation"} {
		t.Run(mode, func(t *testing.T) {
			p := preparedFixture()
			p.Version = "v1.235.0"
			p.Tag = p.Version
			p.Milestone.Title = p.Version
			if mode == "candidate" || mode == "lost reservation" {
				p.Mode = releaseModeCandidate
				p.Tag += "-rc.1"
			}
			parent := taggedRun(p)
			parent.ID = 77
			parent.Path = ".github/workflows/publish-release.yml"
			parent.HeadBranch = "main"
			parent.Actor = buildActor{ID: 52489, Login: "osterman"}
			parent.TriggeringActor = parent.Actor
			a := buildArtifact{ID: 88, Name: "release-plan-77-1"}
			a.WorkflowRun.ID = 77
			a.WorkflowRun.HeadSHA = parent.HeadSHA
			routes := planRoutes()
			routes["milestones"] = []Milestone{p.Milestone}
			pr := routes["pulls/42"].(releasePull)
			pr.Milestone = &p.Milestone
			routes["pulls/42"] = pr
			if mode == "changed milestone" {
				routes["milestones"] = []Milestone{{Number: 1, Title: "v1.236.0", State: "open"}}
			}
			reservation := newCandidateAPI(t)
			if mode != "lost reservation" {
				reservation.add(p)
			}
			reads := map[string]bool{}
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)
				reads[r.URL.Path] = true
				if p.Mode == releaseModeCandidate && reservation.serve(w, r) {
					return
				}
				if r.URL.Path == "/users/osterman" {
					githubtest.JSON(t, w, map[string]any{"id": 52489, "type": "User"})
					return
				}
				if r.URL.Path == "/users/untrusted" {
					githubtest.JSON(t, w, map[string]any{"id": 123, "type": "User"})
					return
				}
				path := planPath(r.URL.Path)
				switch path {
				case "actions/runs/77":
					githubtest.JSON(t, w, parent)
					return
				case "actions/artifacts/88":
					githubtest.JSON(t, w, a)
					return
				}
				if strings.HasPrefix(path, "git/ref/tags/") {
					sha := p.Commit
					if mode == "moved tag" {
						sha = githubtest.Base
					}
					githubtest.JSON(t, w, tagObject("commit", sha))
					return
				}
				value, ok := routes[path]
				require.True(t, ok, "unexpected %s", path)
				githubtest.JSON(t, w, value)
			})
			env := releaseEnvironment()
			env["GITHUB_RUN_ID"] = "99"
			env["GITHUB_ACTOR_ID"] = "41898282"
			env["GITHUB_REF"] = "refs/tags/" + p.Tag
			env["GITHUB_SHA"] = p.Commit
			env["GITHUB_WORKFLOW_SHA"] = p.Commit
			env["GITHUB_WORKFLOW_REF"] = p.Repository + "/" + taggedBuildWorkflow + "@refs/tags/" + p.Tag
			if mode == "unauthorized rerun" {
				env["GITHUB_RUN_ATTEMPT"] = "2"
				env["GITHUB_TRIGGERING_ACTOR"] = "untrusted"
			}
			err := c.validateTagBuild(p, "88", "77", tagBuildRequest(p, "88"), environmentReader(env))
			if mode == "cut" || mode == "candidate" {
				require.NoError(t, err)
				require.True(t, reads[planPrefix+"milestones"])
				require.True(t, reads[planPrefix+"git/ref/tags/"+p.Tag])
			} else {
				require.Error(t, err)
			}
		})
	}
}
