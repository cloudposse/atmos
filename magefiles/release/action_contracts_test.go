package release

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Composite manifests cannot resolve workflow vars or secrets directly. Keep
// this check separate from actionlint, which only validates workflow files.
func TestReleaseCompositeActionsReceiveWorkflowConfigurationExplicitly(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", ".github", "actions", "release-*", "action.yml"))
	require.NoError(t, err)
	require.NotEmpty(t, paths)
	implicit := regexp.MustCompile(`\$\{\{[^}]*\b(vars|secrets)\s*[.\[]`)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		require.NotRegexp(t, implicit, string(data), "%s must receive workflow configuration through inputs", path)
	}
}

// Follow the actual local action graph so a renamed input or missing required
// argument fails locally, before Actions loads a later publication stage.
func TestReleaseLocalActionInputContracts(t *testing.T) {
	var workflow pipelineWorkflow
	pipelineYAML(t, ".github/workflows/release-pipeline.yml", &workflow)
	visited := map[string]bool{}
	var checkSteps func([]pipelineStep)
	checkSteps = func(steps []pipelineStep) {
		for _, step := range steps {
			if !strings.HasPrefix(step.Uses, "./.github/actions/") {
				continue
			}
			var action struct {
				Inputs map[string]struct {
					Required bool    `yaml:"required"`
					Default  *string `yaml:"default"`
				} `yaml:"inputs"`
				Runs struct {
					Steps []pipelineStep `yaml:"steps"`
				} `yaml:"runs"`
			}
			pipelineYAML(t, step.Uses+"/action.yml", &action)
			for name := range step.With {
				_, exists := action.Inputs[name]
				require.True(t, exists, "%s has no input %s", step.Uses, name)
			}
			for name, input := range action.Inputs {
				if input.Required && input.Default == nil {
					require.NotEmpty(t, step.With[name], "%s requires input %s", step.Uses, name)
				}
			}
			if !visited[step.Uses] {
				visited[step.Uses] = true
				checkSteps(action.Runs.Steps)
			}
		}
	}
	for _, job := range workflow.Jobs {
		checkSteps(job.Steps)
	}
	require.NotEmpty(t, visited)
}

func TestReleaseGuardInputReachesEveryAuthoritativeCheck(t *testing.T) {
	var workflow pipelineWorkflow
	pipelineYAML(t, ".github/workflows/release-pipeline.yml", &workflow)
	for job, action := range map[string]string{"prepare": "release-prepare", "verify": "release-verify", "publish": "release-finalize"} {
		t.Run(job, func(t *testing.T) {
			var manifest struct {
				Inputs map[string]struct {
					Required bool `yaml:"required"`
				} `yaml:"inputs"`
			}
			pipelineYAML(t, ".github/actions/"+action+"/action.yml", &manifest)
			require.True(t, manifest.Inputs["guarded-releases-enabled"].Required)
			caller := pipelineUses(t, workflow.Jobs[job].Steps, "./.github/actions/"+action)
			require.Equal(t, "${{ vars.ATMOS_GUARDED_RELEASES_ENABLED }}", caller.With["guarded-releases-enabled"])
			checks := 0
			for _, step := range pipelineAction(t, action) {
				if value, ok := step.Env["ATMOS_GUARDED_RELEASES_ENABLED"]; ok {
					checks++
					require.Equal(t, "${{ inputs.guarded-releases-enabled }}", value)
				}
			}
			require.Positive(t, checks)
		})
	}
}
