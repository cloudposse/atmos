//go:build mage

package qualitygate

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/stretchr/testify/require"
)

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
			require.Contains(t, body, "No merge conflicts with the target branch")
		})
	}
}
