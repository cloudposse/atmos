//go:build mage

package release

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/stretchr/testify/require"
)

func TestReleaseOperatorIdentityExactEquality(t *testing.T) {
	for _, tc := range []struct {
		id      int64
		allowed bool
	}{{52489, true}, {7356997, true}, {0, false}, {-52489, false}, {152489, false}, {524890, false}, {17356997, false}, {73569970, false}} {
		require.Equal(t, tc.allowed, authorizedReleaseID(tc.id), "ID %d", tc.id)
	}
}

func TestReleaseCommandEntrypoints(t *testing.T) {
	for _, mode := range []string{"check", "plan-json", "cut", "candidate"} {
		t.Run(mode, func(t *testing.T) {
			if mode == "cut" {
				setApprovedCut(t, approvedCutFixture())
			} else {
				t.Setenv("RELEASE_CONFIRMED", "false")
				t.Setenv("RELEASE_APPROVED_PLAN", "")
			}
			request := ""
			writes := 0
			routes := planRoutes()
			candidate := newCandidateAPI(t)
			reserved := preparedFixture()
			reserved.Mode, reserved.Tag = "candidate", "v1.234.0-rc.1"
			candidate.add(reserved)
			milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				if mode == "candidate" && candidate.serve(w, r) {
					return
				}
				path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, strings.TrimSuffix(planPrefix, "/")), "/")
				switch {
				case r.URL.Path == "/user":
					githubtest.JSON(t, w, map[string]any{"id": 52489, "type": "User"})
				case path == "actions/workflows/publish-release.yml/dispatches":
					writes++
					require.Equal(t, http.MethodPost, r.Method)
					var body struct {
						Ref    string            `json:"ref"`
						Inputs map[string]string `json:"inputs"`
					}
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					require.Equal(t, "main", body.Ref)
					request = body.Inputs["request-id"]
					require.Regexp(t, "^[a-f0-9]{32}$", request)
					require.Equal(t, map[string]string{"mode": mode, "milestone": "1", "version": "v1.234.0", "commit": githubtest.SHA, "request-id": request}, body.Inputs)
					w.WriteHeader(http.StatusNoContent)
				case path == "actions/workflows/publish-release.yml/runs":
					githubtest.JSON(t, w, map[string]any{"workflow_runs": []map[string]any{{"id": 77, "display_title": "Atmos release " + request, "event": "workflow_dispatch", "html_url": "https://github.com/run/77"}, {"id": 88, "display_title": "Atmos release another-request", "event": "workflow_dispatch"}}})
				case strings.HasPrefix(path, "releases/tags/"):
					tag := "v1.234.0"
					if mode == "candidate" {
						tag += "-rc.1"
					}
					require.Equal(t, "releases/tags/"+tag, path)
					githubtest.JSON(t, w, map[string]any{"tag_name": tag, "target_commitish": githubtest.SHA, "draft": false, "immutable": true, "prerelease": mode == "candidate"})
				case path == "actions/runs/77":
					githubtest.JSON(t, w, map[string]string{"status": "completed", "conclusion": "success", "html_url": "https://github.com/run/77"})
				default:
					require.Equal(t, http.MethodGet, r.Method, "local commands must not create tags, drafts or milestones")
					value, ok := routes[path]
					require.True(t, ok, "unexpected %s", r.URL)
					githubtest.JSON(t, w, value)
				}
			})
			var err error
			switch mode {
			case "check":
				err = (Release{}).Check(githubtest.Repo)
			case "plan-json":
				err = (Release{}).PlanJSON(githubtest.Repo)
			case "cut":
				err = (Release{}).Cut(githubtest.Repo)
			case "candidate":
				err = (Release{}).Candidate(githubtest.Repo)
			}
			require.NoError(t, err)
			if mode == "check" || mode == "plan-json" {
				require.Zero(t, writes)
			} else {
				require.Equal(t, 1, writes)
			}
		})
	}
	require.ErrorIs(t, (Release{}).Check("invalid"), ErrMilestonePolicy)
}

func TestReleaseCommandFailuresDoNotDispatch(t *testing.T) {
	setApprovedCut(t, approvedCutFixture())
	for _, mode := range []string{"plan", "output", "user API", "bot", "substring identity", "unknown identity", "dispatch"} {
		t.Run(mode, func(t *testing.T) {
			routes := planRoutes()
			writes := 0
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/user" {
					respondCommandUser(t, w, mode)
					return
				}
				path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, strings.TrimSuffix(planPrefix, "/")), "/")
				if r.Method != http.MethodGet {
					writes++
					require.Equal(t, "dispatch", mode)
					http.Error(w, "lost dispatch response", http.StatusServiceUnavailable)
					return
				}
				if mode == "plan" && path == "milestones" {
					http.Error(w, "cannot plan", http.StatusServiceUnavailable)
					return
				}
				value, ok := routes[path]
				require.True(t, ok, "unexpected %s", r.URL)
				githubtest.JSON(t, w, value)
			})
			var output bytes.Buffer
			var out io.Writer = &output
			if mode == "output" {
				reader, writer := io.Pipe()
				require.NoError(t, reader.Close())
				t.Cleanup(func() { _ = writer.Close() })
				out = writer
			}
			err := c.command("cut", out, func(context.Context) error { t.Error("unexpected poll"); return errors.New("unexpected poll") })
			require.Error(t, err)
			if mode == "dispatch" {
				require.Equal(t, 1, writes)
				require.ErrorContains(t, err, "dispatch result uncertain for request")
				require.ErrorContains(t, err, "before retrying")
			} else {
				require.Zero(t, writes)
			}
		})
	}
}

func TestReleaseCheckHumanOutputAndCapturedPlan(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	c := planHTTP(t, planRoutes(), "unreachable")
	var summary bytes.Buffer
	noPoll := func(context.Context) error { t.Fatal("check must not dispatch or poll"); return nil }
	require.NoError(t, c.command("check", &summary, noPoll))
	require.Equal(t, "Release 1.234.0\n\n  Source    main @ "+githubtest.SHA+"\n  Previous  v1.233.0\n\n✓ Milestone and source checks passed.\n", summary.String())
	require.NotContains(t, summary.String(), "PR body is data")
	require.NotContains(t, summary.String(), `"entries"`)

	var captured bytes.Buffer
	require.NoError(t, c.command("plan-json", &captured, noPoll))
	var plan Plan
	require.NoError(t, json.Unmarshal(captured.Bytes(), &plan), "confirmation must still receive an intact machine-readable plan")
	require.Equal(t, "v1.234.0", plan.Version)
	require.Equal(t, githubtest.SHA, plan.Commit)
	require.Equal(t, "cut", plan.Mode)
	require.Len(t, plan.Entries, 2)
	require.Equal(t, "PR body is data", plan.Entries[0].Body)
	require.NotContains(t, captured.String(), "checks passed")
}

func TestReleaseCheckFailurePrintsNoSuccess(t *testing.T) {
	c := planHTTP(t, planRoutes(), "milestones")
	var output bytes.Buffer
	err := c.command("check", &output, func(context.Context) error { t.Fatal("unexpected poll"); return nil })
	require.Error(t, err)
	require.Empty(t, output.String())
}

func TestFollowReleaseRunCorrelationAndPolling(t *testing.T) {
	listings, polls, statusReads := 0, 0, 0
	c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, strings.TrimSuffix(planPrefix, "/")), "/")
		if path == "actions/workflows/publish-release.yml/runs" {
			listings++
			require.Equal(t, "workflow_dispatch", r.URL.Query().Get("event"))
			runs := []map[string]any{{"id": 1, "display_title": "Atmos release requested-extra", "event": "workflow_dispatch"}, {"id": 2, "display_title": "Atmos release requested", "event": "schedule"}}
			if listings > 1 {
				runs = append(runs, map[string]any{"id": 77, "display_title": "Atmos release requested", "event": "workflow_dispatch", "html_url": "https://github.com/run/77"})
			}
			githubtest.JSON(t, w, map[string]any{"workflow_runs": runs})
			return
		}
		require.Equal(t, "actions/runs/77", path)
		statusReads++
		status := "in_progress"
		if statusReads == 2 {
			status = "completed"
		}
		githubtest.JSON(t, w, map[string]string{"status": status, "conclusion": "success", "html_url": "https://github.com/run/77"})
	})
	var out bytes.Buffer
	runID, err := c.followRun("requested", &out, func(context.Context) error { polls++; return nil })
	require.NoError(t, err)
	require.EqualValues(t, 77, runID)
	require.Equal(t, 2, polls)
	require.Equal(t, 2, listings)
	require.Equal(t, 2, statusReads)
	require.Contains(t, out.String(), "Release workflow succeeded: https://github.com/cloudposse/atmos/actions/runs/77")
}

func TestFollowReleaseRunFailures(t *testing.T) {
	for _, mode := range []string{"context cancelled", "list API", "duplicate identity", "run API", "failed run", "cancelled run", "poll cancelled"} {
		t.Run(mode, func(t *testing.T) {
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, strings.TrimSuffix(planPrefix, "/")), "/")
				if path == "actions/workflows/publish-release.yml/runs" {
					if mode == "list API" {
						http.Error(w, "list unavailable", http.StatusServiceUnavailable)
						return
					}
					runs := []map[string]any{{"id": 77, "display_title": "Atmos release request", "event": "workflow_dispatch"}}
					if mode == "duplicate identity" {
						runs = append(runs, map[string]any{"id": 88, "display_title": "Atmos release request", "event": "workflow_dispatch"})
					}
					if mode == "poll cancelled" {
						runs = nil
					}
					githubtest.JSON(t, w, map[string]any{"workflow_runs": runs})
					return
				}
				if mode == "run API" {
					http.Error(w, "run unavailable", http.StatusServiceUnavailable)
					return
				}
				conclusion := "failure"
				if mode == "cancelled run" {
					conclusion = "cancelled"
				}
				githubtest.JSON(t, w, map[string]string{"status": "completed", "conclusion": conclusion, "html_url": "https://github.com/run/77"})
			})
			if mode == "context cancelled" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				c.Context = ctx
			}
			_, err := c.followRun("request", io.Discard, func(context.Context) error { return context.Canceled })
			require.Error(t, err)
			switch mode {
			case "context cancelled", "poll cancelled":
				require.ErrorIs(t, err, context.Canceled)
				require.Contains(t, err.Error(), "request")
			case "duplicate identity":
				require.ErrorIs(t, err, ErrMilestonePolicy)
			case "failed run":
				require.ErrorContains(t, err, "finished failure")
			case "cancelled run":
				require.ErrorContains(t, err, "finished cancelled")
			}
		})
	}
}

func TestFollowReleaseRunPagination(t *testing.T) {
	for _, exhaustive := range []bool{false, true} {
		t.Run(fmt.Sprint(exhaustive), func(t *testing.T) {
			calls := 0
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/runs/77") {
					githubtest.JSON(t, w, map[string]string{"status": "completed", "conclusion": "success"})
					return
				}
				calls++
				require.Equal(t, fmt.Sprint(calls), r.URL.Query().Get("page"))
				runs := make([]map[string]any, 100)
				for i := range runs {
					runs[i] = map[string]any{"id": i + 100, "display_title": "unrelated", "event": "workflow_dispatch"}
				}
				if !exhaustive && calls == 2 {
					runs = []map[string]any{{"id": 77, "display_title": "Atmos release request", "event": "workflow_dispatch"}}
				}
				githubtest.JSON(t, w, map[string]any{"workflow_runs": runs})
			})
			_, err := c.followRun("request", io.Discard, func(context.Context) error { return context.Canceled })
			if exhaustive {
				require.ErrorIs(t, err, context.Canceled)
				require.Equal(t, 100, calls)
			} else {
				require.NoError(t, err)
				require.Equal(t, 2, calls)
			}
		})
	}
}

func TestReleasePollCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, waitReleasePoll(ctx), context.Canceled)
}

func approvedCutFixture() *Plan {
	return &Plan{Repository: githubtest.Repo, Milestone: Milestone{Number: 1, Title: "1.234.0", State: "open"}, Version: "v1.234.0", Commit: githubtest.SHA, PreviousTag: "v1.233.0", Branch: "main", Mode: "cut"}
}

func setApprovedCut(t *testing.T, p *Plan) {
	t.Helper()
	data, err := json.Marshal(p)
	require.NoError(t, err)
	t.Setenv("RELEASE_CONFIRMED", "true")
	t.Setenv("RELEASE_APPROVED_PLAN", string(data))
}

func TestReleaseCutRequiresExplicitConfirmationBeforeAnyAPI(t *testing.T) {
	for _, value := range []string{"", "false", "no", "TRUE", "1"} {
		t.Run("confirmation="+value, func(t *testing.T) {
			setApprovedCut(t, approvedCutFixture())
			t.Setenv("RELEASE_CONFIRMED", value)
			calls := 0
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				t.Errorf("declined or absent confirmation must not access GitHub: %s", r.URL)
				http.Error(w, "unexpected API request", http.StatusInternalServerError)
			})
			err := c.command("cut", io.Discard, func(context.Context) error { t.Error("unexpected poll"); return context.Canceled })
			require.ErrorIs(t, err, ErrMilestonePolicy)
			require.ErrorContains(t, err, "release was not confirmed")
			require.Zero(t, calls)
		})
	}
}

func TestReleaseCutRejectsMissingStaleOrChangedApproval(t *testing.T) {
	for _, mode := range []string{"missing approval", "malformed JSON", "null approval", "wrong repository", "wrong version", "wrong SHA", "wrong milestone ID", "renamed milestone", "wrong baseline", "wrong branch", "wrong mode", "next milestone changed"} {
		t.Run(mode, func(t *testing.T) {
			approved := approvedCutFixture()
			switch mode {
			case "wrong repository":
				approved.Repository = "other/atmos"
			case "wrong version":
				approved.Version = "v1.235.0"
			case "wrong SHA":
				approved.Commit = githubtest.Base
			case "wrong milestone ID":
				approved.Milestone.Number = 2
			case "renamed milestone":
				approved.Milestone.Title = "v1.234.0"
			case "wrong baseline":
				approved.PreviousTag = "v1.232.0"
			case "wrong branch":
				approved.Branch = "release"
			case "wrong mode":
				approved.Mode = "candidate"
			}
			setApprovedCut(t, approved)
			switch mode {
			case "missing approval":
				t.Setenv("RELEASE_APPROVED_PLAN", "")
			case "malformed JSON":
				t.Setenv("RELEASE_APPROVED_PLAN", "{")
			case "null approval":
				t.Setenv("RELEASE_APPROVED_PLAN", "null")
			}
			routes := planRoutes()
			if mode == "next milestone changed" {
				// A manual patch milestone becomes the next release after the operator
				// approved the previously lowest minor milestone. Eligibility still passes.
				m := Milestone{Number: 7, Title: "v1.233.1", State: "open"}
				routes["milestones"] = []Milestone{m, {Number: 1, Title: "1.234.0", State: "open"}}
				pr := routes["pulls/42"].(releasePull)
				pr.Milestone = &m
				routes["pulls/42"] = pr
			}
			writes, userReads := 0, 0
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					writes++
					http.Error(w, "unexpected dispatch", http.StatusInternalServerError)
					return
				}
				if r.URL.Path == "/user" {
					userReads++
					http.Error(w, "unexpected user lookup", http.StatusInternalServerError)
					return
				}
				value, ok := routes[strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, strings.TrimSuffix(planPrefix, "/")), "/")]
				require.True(t, ok, "unexpected %s", r.URL)
				githubtest.JSON(t, w, value)
			})
			err := c.command("cut", io.Discard, func(context.Context) error { t.Error("unexpected poll"); return context.Canceled })
			require.ErrorIs(t, err, ErrMilestonePolicy)
			if mode == "missing approval" || mode == "malformed JSON" {
				require.ErrorContains(t, err, "confirmed release plan missing or invalid")
			} else {
				require.ErrorContains(t, err, "release plan changed after confirmation")
			}
			require.Zero(t, writes)
			require.Zero(t, userReads, "the user lookup must not begin until the approved live plan matches")
		})
	}
}

func TestApprovalCannotAuthorizeCandidateAsCut(t *testing.T) {
	approved := approvedCutFixture()
	data, err := json.Marshal(approved)
	require.NoError(t, err)
	current := *approved
	current.Mode = "candidate"
	require.ErrorIs(t, validateApprovedPlan(string(data), &current), ErrMilestonePolicy)
}

func TestReleaseCompletionSummary(t *testing.T) {
	for _, mode := range []string{"cut", "candidate"} {
		t.Run(mode, func(t *testing.T) {
			p := approvedCutFixture()
			p.Mode = mode
			tag := p.Version
			if mode == "candidate" {
				tag += "-rc.1"
			}
			candidate := newCandidateAPI(t)
			candidate.add(&preparedRelease{Plan: *p, Tag: tag, RunID: "77"})
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				if mode == "candidate" && candidate.serve(w, r) {
					return
				}
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, planPrefix+"releases/tags/"+tag, r.URL.Path)
				githubtest.JSON(t, w, map[string]any{"tag_name": tag, "target_commitish": p.Commit, "draft": false, "immutable": true, "prerelease": mode == "candidate"})
			})
			var out bytes.Buffer
			require.NoError(t, c.summarizeRelease(p, 77, &out))
			require.Equal(t, "Published release: "+tag+"\nVersion: "+strings.TrimPrefix(tag, "v")+"\nCommit: "+p.Commit+"\nRelease: https://github.com/cloudposse/atmos/releases/tag/"+tag+"\nWorkflow: https://github.com/cloudposse/atmos/actions/runs/77\n", out.String())
		})
	}
}

func TestReleaseCompletionDoesNotClaimUnverifiedPublication(t *testing.T) {
	for _, mode := range []string{"API failure", "wrong tag", "wrong SHA", "draft", "mutable", "prerelease", "output failure"} {
		t.Run(mode, func(t *testing.T) {
			p := approvedCutFixture()
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, planPrefix+"releases/tags/v1.234.0", r.URL.Path)
				if mode == "API failure" {
					http.Error(w, "release not found", http.StatusNotFound)
					return
				}
				published := map[string]any{"tag_name": p.Version, "target_commitish": p.Commit, "draft": false, "immutable": true, "prerelease": false}
				switch mode {
				case "wrong tag":
					published["tag_name"] = "v1.235.0"
				case "wrong SHA":
					published["target_commitish"] = githubtest.Base
				case "draft":
					published["draft"] = true
				case "mutable":
					published["immutable"] = false
				case "prerelease":
					published["prerelease"] = true
				}
				githubtest.JSON(t, w, published)
			})
			var out bytes.Buffer
			var writer io.Writer = &out
			if mode == "output failure" {
				reader, pipe := io.Pipe()
				require.NoError(t, reader.Close())
				t.Cleanup(func() { _ = pipe.Close() })
				writer = pipe
			}
			err := c.summarizeRelease(p, 77, writer)
			require.Error(t, err)
			require.Empty(t, out.String())
			if mode == "output failure" {
				require.ErrorIs(t, err, io.ErrClosedPipe)
			} else {
				require.ErrorContains(t, err, "https://github.com/cloudposse/atmos/actions/runs/77")
			}
		})
	}
}

func TestKnownReleaseRunCancellationIncludesExactRunURL(t *testing.T) {
	for _, atPause := range []bool{false, true} {
		t.Run(fmt.Sprint(atPause), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			requests := 0
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				requests++
				require.Equal(t, http.MethodGet, r.Method)
				if strings.HasSuffix(r.URL.Path, "/runs/77") {
					githubtest.JSON(t, w, map[string]string{"status": "in_progress"})
					return
				}
				githubtest.JSON(t, w, map[string]any{"workflow_runs": []map[string]any{{"id": 77, "display_title": "Atmos release request", "event": "workflow_dispatch"}}})
			})
			c.Context = ctx
			id, err := c.followRun("request", io.Discard, func(context.Context) error {
				cancel()
				if atPause {
					return context.Canceled
				}
				return nil
			})
			require.EqualValues(t, 77, id)
			require.ErrorIs(t, err, context.Canceled)
			require.ErrorContains(t, err, "https://github.com/cloudposse/atmos/actions/runs/77")
			require.Equal(t, 2, requests)
		})
	}
}

func TestReleaseCommandDoesNotRedispatchFailedWorkflow(t *testing.T) {
	setApprovedCut(t, approvedCutFixture())
	routes := planRoutes()
	request := ""
	dispatches := 0
	c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, strings.TrimSuffix(planPrefix, "/")), "/")
		switch {
		case r.URL.Path == "/user":
			githubtest.JSON(t, w, map[string]any{"id": 52489, "type": "User"})
		case strings.HasSuffix(path, "/dispatches"):
			dispatches++
			require.Equal(t, http.MethodPost, r.Method)
			var body struct {
				Inputs map[string]string `json:"inputs"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			request = body.Inputs["request-id"]
			w.WriteHeader(http.StatusNoContent)
		case path == "actions/workflows/publish-release.yml/runs":
			githubtest.JSON(t, w, map[string]any{"workflow_runs": []map[string]any{{"id": 77, "display_title": "Atmos release " + request, "event": "workflow_dispatch"}}})
		case path == "actions/runs/77":
			githubtest.JSON(t, w, map[string]string{"status": "completed", "conclusion": "failure"})
		default:
			value, ok := routes[path]
			require.True(t, ok, "failed workflow must not fetch or claim published release: %s", path)
			githubtest.JSON(t, w, value)
		}
	})
	var out bytes.Buffer
	err := c.command("cut", &out, func(context.Context) error { t.Fatal("completed failure must not poll again"); return nil })
	require.ErrorContains(t, err, "finished failure")
	require.ErrorContains(t, err, "https://github.com/cloudposse/atmos/actions/runs/77")
	require.Equal(t, 1, dispatches)
	require.NotContains(t, out.String(), "Published release:")
}

func respondCommandUser(t *testing.T, w http.ResponseWriter, mode string) {
	t.Helper()
	if mode == "user API" {
		http.Error(w, "user unavailable", http.StatusServiceUnavailable)
		return
	}
	id, kind := int64(52489), "User"
	switch mode {
	case "bot":
		kind = "Bot"
	case "substring identity":
		id = 152489
	case "unknown identity":
		id = 123
	}
	githubtest.JSON(t, w, map[string]any{"id": id, "type": kind})
}
