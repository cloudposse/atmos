//go:build mage

package qualitygate

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/stretchr/testify/require"
)

// TestQualityStackedPublication permits eligible stacked PRs and rejects metadata changes before publication.
func TestQualityStackedPublication(t *testing.T) {
	for _, scenario := range []string{"stacked", "closed", "foreign repository", "missing base", "retargeted", "base advanced", "repository changed"} {
		t.Run(scenario, func(t *testing.T) {
			run := githubtest.Run()
			pr := githubtest.PR(run)
			count := 10
			pr.ChangedFiles = &count
			pr.Base.Ref = "feature/prerequisite"
			switch scenario {
			case "closed":
				pr.State = "closed"
			case "foreign repository":
				pr.Base.Repo.FullName = "another/repository"
			case "missing base":
				pr.Base.Ref = ""
			}
			reads, writes := 0, 0
			getter := qualityPublishGetter(t, "create", &pr, &run, &reads)
			fake := &githubtest.FakeAPI{T: t}
			fake.Get = func(path string, response any) error {
				if strings.HasSuffix(path, "/pulls/42") {
					reads++
					current := pr
					if reads > 1 {
						switch scenario {
						case "retargeted":
							current.Base.Ref = "main"
						case "base advanced":
							current.Base.SHA = githubtest.SHA
						case "repository changed":
							current.Base.Repo.FullName = "another/repository"
						}
					}
					return githubtest.Respond(current, response)
				}
				return getter(path, response)
			}
			var body string
			fake.Write = func(_ string, path string, input io.Reader) error {
				writes++
				if strings.Contains(path, "comments") {
					var payload map[string]string
					require.NoError(t, json.NewDecoder(input).Decode(&payload))
					body = payload["body"]
				}
				return nil
			}
			q := qualityClient{ctx: context.Background(), api: &qualityGraphAPI{FakeAPI: fake, t: t, scenario: "valid"}, repo: githubtest.Repo}
			require.NoError(t, q.refresh(pr.Number, "", nil))
			if scenario != "stacked" {
				require.Zero(t, writes)
				return
			}
			require.Equal(t, 2, writes)
			require.Contains(t, body, "Quality Gate passed")
			require.Contains(t, body, qualityHeadMarker(pr.Head.SHA))
			require.Contains(t, body, "<!-- atmos-quality-base:feature%2Fprerequisite:"+pr.Base.SHA+" -->")
			require.Contains(t, body, "No merge conflicts with the target branch")
		})
	}
}

// TestQualityScheduledBaseChanges refreshes old reports when the base changes without head activity.
func TestQualityScheduledBaseChanges(t *testing.T) {
	cutoff := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	for _, scenario := range []string{"unchanged", "base advanced", "retargeted", "legacy report"} {
		t.Run(scenario, func(t *testing.T) {
			pr := githubtest.PR(githubtest.Run())
			pr.Base.Ref = "feature/prerequisite"
			pr.Updated = cutoff.Add(-time.Hour)
			pr.Labels = append(pr.Labels, struct {
				Name string `json:"name"`
			}{Name: qualityReady})
			body, ready := qualityMarkdown(&pr, nil, "Performance measurements pending for this commit.")
			require.True(t, ready)
			switch scenario {
			case "base advanced":
				pr.Base.SHA = githubtest.SHA
			case "retargeted":
				pr.Base.Ref = "main"
			case "legacy report":
				body = qualityMarker + "\n" + qualityHeadMarker(pr.Head.SHA)
			}
			comment := githubapi.Comment{ID: 17, Body: body}
			comment.User.Login, comment.User.Type = "github-actions[bot]", "Bot"
			q := qualityTestClient(t, func(path string, response any) error {
				switch {
				case strings.Contains(path, "/pulls?"):
					return githubtest.Respond([]githubapi.PR{pr}, response)
				case strings.Contains(path, "/issues/42/comments?"):
					return githubtest.Respond([]githubapi.Comment{comment}, response)
				default:
					t.Fatalf("unexpected API read: %s", path)
					return nil
				}
			})
			selected, err := q.recentPRNumbers(cutoff)
			require.NoError(t, err)
			if scenario == "unchanged" {
				require.Empty(t, selected)
			} else {
				require.Equal(t, []int{pr.Number}, selected)
			}
		})
	}
}

// TestQualityUnavailableEvidenceAfterBaseChange prevents stale readiness from surviving a target revision change.
func TestQualityUnavailableEvidenceAfterBaseChange(t *testing.T) {
	for _, scenario := range []string{"unchanged", "base advanced", "retargeted", "legacy report"} {
		t.Run(scenario, func(t *testing.T) {
			state := &stateFixture{labels: []string{qualityReady}, reviewsFail: true}
			pr := state.pr()
			previous := pr
			switch scenario {
			case "base advanced":
				previous.Base.SHA = githubtest.SHA
			case "retargeted":
				previous.Base.Ref = "feature/prerequisite"
			}
			state.comment, _ = qualityMarkdown(&previous, nil, "Performance measurements pending for this commit.")
			if scenario == "legacy report" {
				state.comment = qualityMarker + "\n" + qualityHeadMarker(pr.Head.SHA) + "\nPassed report\n"
			}
			err := state.client(t).refresh(pr.Number, "", nil)
			if scenario == "unchanged" {
				require.ErrorContains(t, err, "evidence unavailable")
				require.Empty(t, state.writes)
				return
			}
			require.NoError(t, err)
			require.Equal(t, []string{
				"DELETE repos/" + githubtest.Repo + "/issues/42/labels/" + qualityReady,
				"POST repos/" + githubtest.Repo + "/issues/42/labels",
				"PATCH repos/" + githubtest.Repo + "/issues/comments/17",
			}, state.writes)
			require.Len(t, state.comments, 1)
			require.Contains(t, state.comments[0], "Quality Gate not ready")
			require.Contains(t, state.comments[0], "Unavailable: reviews unavailable")
			require.Contains(t, state.comments[0], qualityBaseMarker(&pr))
		})
	}
}
