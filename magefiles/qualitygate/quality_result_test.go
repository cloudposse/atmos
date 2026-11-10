//go:build mage

package qualitygate

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/stretchr/testify/require"
)

func TestQualityResultStates(t *testing.T) {
	pr := githubtest.PR(githubtest.Run())
	for _, tc := range []struct {
		name, state string
		gates       []qualityGate
		pending     bool
	}{
		{"ready", "ready", []qualityGate{{Passed: true}}, false},
		{"blocked", "blocked", []qualityGate{{Passed: false}}, false},
		{"CI pending", "pending", []qualityGate{{Passed: true}}, true},
		{"unavailable", "pending", []qualityGate{{Unavailable: true}}, false},
		{"external checks running", "pending", []qualityGate{{Signals: []qualitySignal{{Name: "external", State: "in_progress"}}}}, false},
		{"advisory", "ready", []qualityGate{{Advisory: true, Unavailable: true}}, false},
		{"no evidence", "pending", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := qualitySnapshot(&pr, tc.gates, tc.pending)
			require.Equal(t, tc.state, result.State)
			body, _ := qualityMarkdown(&pr, tc.gates, "", result)
			parsed, valid := qualityReadResult(body, &pr)
			require.True(t, valid)
			require.Equal(t, result, parsed)
		})
	}
}

func TestQualityResultRejectsOtherRevisionsAndMalformedRecords(t *testing.T) {
	pr := githubtest.PR(githubtest.Run())
	result := qualitySnapshot(&pr, []qualityGate{{Passed: true}}, false)
	for _, tc := range []struct {
		name   string
		change func(*githubapi.PR)
	}{
		{"head", func(p *githubapi.PR) { p.Head.SHA = githubtest.Base }},
		{"base", func(p *githubapi.PR) { p.Base.SHA = githubtest.SHA }},
		{"retarget", func(p *githubapi.PR) { p.Base.Ref = "feature/parent" }},
		{"draft", func(p *githubapi.PR) { p.Draft = !p.Draft }},
		{"milestone", func(p *githubapi.PR) { p.Milestone = &githubapi.PRMilestone{Number: 99} }},
		{"release label", func(p *githubapi.PR) {
			p.Labels = append(p.Labels, struct {
				Name string `json:"name"`
			}{Name: "major"})
		}},
		{"other PR", func(p *githubapi.PR) { p.Number++ }},
		{"other repo", func(p *githubapi.PR) { p.Base.Repo.FullName = "other/repo" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := pr
			tc.change(&changed)
			_, valid := qualityReadResult(qualityReportHeader+qualityResultMarker(&result)+"\n", &changed)
			require.False(t, valid)
		})
	}
	for _, body := range []string{
		qualityMarker, qualityReportHeader + qualityResultPrefix + "{ -->\n",
		qualityReportHeader + "third-party text\n" + qualityResultMarker(&result) + "\n",
		qualityReportHeader + qualityResultPrefix + `{}`,
	} {
		_, valid := qualityReadResult(body, &pr)
		require.False(t, valid)
	}
	for _, invalid := range []qualityResult{{SchemaVersion: 2}, {State: "success"}} {
		bad := result
		if invalid.SchemaVersion != 0 {
			bad.SchemaVersion = invalid.SchemaVersion
		} else {
			bad.State = invalid.State
		}
		_, valid := qualityReadResult(qualityReportHeader+qualityResultMarker(&bad)+"\n", &pr)
		require.False(t, valid)
	}
}

func TestQualityStatusUsesTrustedSnapshotAndLiveCI(t *testing.T) {
	for _, scenario := range []string{"ready", "blocked", "missing", "spoofed", "stale", "rerun", "cancelled test", "cancelled reporter", "external pending", "head moved", "API error", "closed"} {
		t.Run(scenario, func(t *testing.T) {
			run := githubtest.Run()
			run.Event, run.Path = "pull_request", ".github/workflows/test.yml"
			pr := githubtest.PR(run)
			pr.MergeSHA = ""
			result := qualitySnapshot(&pr, []qualityGate{{Passed: scenario != "blocked"}}, false)
			comment := githubapi.Comment{ID: 17, Body: qualityReportHeader + qualityResultMarker(&result) + "\n"}
			comment.User.Login, comment.User.Type = "github-actions[bot]", "Bot"
			reads := 0
			q := qualityTestClient(t, func(path string, response any) error {
				switch {
				case strings.HasSuffix(path, "pulls/42"):
					reads++
					current := pr
					if scenario == "closed" {
						current.State = "closed"
					}
					if scenario == "stale" || (scenario == "head moved" && reads > 1) {
						current.Head.SHA = githubtest.Base
					}
					return githubtest.Respond(current, response)
				case strings.Contains(path, "/comments?"):
					if scenario == "API error" {
						return errors.New("API unavailable")
					}
					if scenario == "missing" {
						return githubtest.Respond([]githubapi.Comment{}, response)
					}
					if scenario == "spoofed" {
						comment.User.Login = "contributor"
					}
					return githubtest.Respond([]githubapi.Comment{comment}, response)
				case strings.Contains(path, "actions/runs?"):
					if scenario == "rerun" {
						run.Status, run.Conclusion = "in_progress", ""
					}
					if scenario == "cancelled test" {
						run.Conclusion = "cancelled"
					}
					runs := []githubapi.Run{run}
					if scenario == "cancelled reporter" {
						reporter := run
						reporter.ID++
						reporter.Path, reporter.Event, reporter.Conclusion = qualityReportWorkflow, "workflow_dispatch", "cancelled"
						runs = append(runs, reporter)
					}
					return githubtest.Respond(map[string]any{"workflow_runs": runs}, response)
				case strings.Contains(path, "/check-runs?"):
					return githubtest.Respond(map[string]any{"check_runs": []any{}}, response)
				case strings.Contains(path, "/statuses?"):
					statuses := []map[string]any{}
					if scenario == "external pending" {
						statuses = append(statuses, map[string]any{"context": "external", "state": "pending"})
					}
					return githubtest.Respond(statuses, response)
				default:
					t.Fatalf("unexpected read: %s", path)
					return nil
				}
			})
			got, err := q.result(42)
			if scenario == "API error" {
				require.ErrorContains(t, err, "API unavailable")
				require.Empty(t, got.State)
				return
			}
			require.NoError(t, err)
			want := "pending"
			if scenario == "ready" || scenario == "cancelled reporter" {
				want = "ready"
			}
			if scenario == "blocked" || scenario == "cancelled test" {
				want = "blocked"
			}
			require.Equal(t, want, got.State)
			if scenario == "head moved" {
				require.Equal(t, "pr_changed_during_read", got.Reason)
				require.Equal(t, githubtest.Base, got.HeadSHA)
			}
		})
	}
}

func TestQualityStatusEntrypointPrintsOnlyJSON(t *testing.T) {
	githubtest.Server(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/repos/cloudposse/atmos/pulls/42", r.URL.Path)
		pr := githubtest.PR(githubtest.Run())
		pr.State = "closed"
		githubtest.JSON(t, w, pr)
	})
	for _, number := range []string{"", "0", "-1", "1; false"} {
		require.ErrorContains(t, (Quality{}).Status(githubtest.Repo, number), "invalid PR number")
	}
	require.ErrorContains(t, (Quality{}).Status("invalid", "42"), "invalid repository")
	output := captureStdout(t, func() { require.NoError(t, (Quality{}).Status(githubtest.Repo, "42")) })
	var result qualityResult
	require.NoError(t, json.Unmarshal([]byte(output), &result))
	require.Equal(t, "pending", result.State)
	require.Equal(t, "pr_not_open_or_eligible", result.Reason)
}
