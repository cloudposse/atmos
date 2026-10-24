package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	atmosExec "github.com/cloudposse/atmos/internal/exec"
	steps "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/template/delimiters"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

type distributionStep struct {
	Name string            `yaml:"name"`
	Uses string            `yaml:"uses"`
	With map[string]string `yaml:"with"`
	Env  map[string]string `yaml:"env"`
	Run  string            `yaml:"run"`
}

type distributionJob struct {
	If          string             `yaml:"if"`
	Needs       string             `yaml:"needs"`
	Environment string             `yaml:"environment"`
	Permissions map[string]string  `yaml:"permissions"`
	Steps       []distributionStep `yaml:"steps"`
}

func readDistributionWorkflow(t *testing.T) map[string]distributionJob {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "release-distribution.yml"))
	require.NoError(t, err)
	var workflow struct {
		Jobs map[string]distributionJob `yaml:"jobs"`
	}
	require.NoError(t, yaml.Unmarshal(data, &workflow))
	require.NotEmpty(t, workflow.Jobs)
	return workflow.Jobs
}

func TestHomebrewDistributionRequiresVerifiedStableRelease(t *testing.T) {
	jobs := readDistributionWorkflow(t)
	brew, ok := jobs["homebrew"]
	require.True(t, ok)
	require.Equal(t, "${{ github.event.release.prerelease == false }}", brew.If, "candidate, nightly and feature releases must never update Homebrew core")
	require.Equal(t, "release", brew.Needs)
	release, ok := jobs[brew.Needs]
	require.True(t, ok)
	require.Equal(t, "verify", release.Needs, "Homebrew must transitively depend on verification")
	verification, ok := jobs[release.Needs]
	require.True(t, ok)
	found := false
	for _, step := range verification.Steps {
		if strings.Contains(step.Uses, "/.github/actions/release-publish") {
			found = true
			require.Equal(t, "verify", step.With["operation"])
			require.Equal(t, "${{ github.event.release.id }}", step.With["release-id"])
		}
	}
	require.True(t, found, "distribution must verify the exact published release and attached asset digests")
	require.Equal(t, "release", brew.Environment)
	require.Equal(t, "read", brew.Permissions["contents"])
}

func TestHomebrewFormulaUsesPublishedTagAndKnownRequiredSetup(t *testing.T) {
	jobs := readDistributionWorkflow(t)
	var bump *distributionStep
	for i := range jobs["homebrew"].Steps {
		step := jobs["homebrew"].Steps[i]
		if strings.Contains(step.Run, "brew bump-formula-pr") {
			require.Nil(t, bump, "one formula update per release")
			bump = &step
		}
	}
	require.NotNil(t, bump)
	require.Equal(t, "${{ secrets.GH_BOT_TOKEN }}", bump.Env["HOMEBREW_GITHUB_API_TOKEN"])
	require.Equal(t, "${{ github.event.release.tag_name }}", bump.Env["TAG"])
	require.Equal(t, "1", bump.Env["HOMEBREW_NO_AUTO_UPDATE"])
	require.Empty(t, bump.Env["HOMEBREW_NO_INSTALL_FROM_API"])
	commands := []string{}
	for _, line := range strings.Split(bump.Run, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			commands = append(commands, line)
		}
	}
	require.Equal(t, []string{
		"set -euo pipefail",
		`eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"`,
		"brew tap homebrew/core --force",
		`brew bump-formula-pr --no-browse --no-audit --version="${TAG#v}" atmos`,
	}, commands, "Homebrew needs PATH initialization and a forced core tap before editing the formula")
	require.NotContains(t, bump.Run, "${{", "release values must travel through environment variables, never shell source interpolation")
}

func TestNativeReleaseCutConfirmsDisplayedPlanBeforeDispatch(t *testing.T) {
	type step struct {
		Name        string            `yaml:"name"`
		Type        string            `yaml:"type"`
		Command     string            `yaml:"command"`
		Script      string            `yaml:"script"`
		Interpreter string            `yaml:"interpreter"`
		Prompt      string            `yaml:"prompt"`
		Default     string            `yaml:"default"`
		Env         map[string]string `yaml:"env"`
	}
	type command struct {
		Name  string `yaml:"name"`
		Steps []step `yaml:"steps"`
	}
	var config struct {
		Commands []struct {
			Name     string    `yaml:"name"`
			Commands []command `yaml:"commands"`
		} `yaml:"commands"`
	}
	data, err := os.ReadFile(filepath.Join("..", "..", ".atmos.d", "release.yaml"))
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(data, &config))
	var cut *command
	for _, root := range config.Commands {
		if root.Name == "release" {
			for _, candidate := range root.Commands {
				if candidate.Name == "cut" {
					copy := candidate
					cut = &copy
				}
			}
		}
	}
	require.NotNil(t, cut)
	require.Len(t, cut.Steps, 3)
	require.Equal(t, "plan", cut.Steps[0].Name)
	require.Equal(t, "script", cut.Steps[0].Type)
	require.Equal(t, "starlark", cut.Steps[0].Interpreter)
	require.Equal(t, `result = exec.run(["go", "tool", "mage", "release:planJSON", "cloudposse/atmos"], output="capture")
output = result.stdout
`, cut.Steps[0].Script)
	require.Equal(t, "confirm", cut.Steps[1].Name)
	require.Equal(t, "confirm", cut.Steps[1].Type)
	require.Equal(t, "no", cut.Steps[1].Default, "declines and unattended/noninteractive execution must not default to release approval")
	require.Contains(t, cut.Steps[1].Prompt, `Do you want to cut {{ trimPrefix "v" (fromJson .steps.plan.value).version }}?`)
	require.Equal(t, "shell", cut.Steps[2].Type)
	require.Equal(t, "go tool mage release:cut cloudposse/atmos", cut.Steps[2].Command)
	require.Equal(t, map[string]string{"RELEASE_CONFIRMED": "{{ .steps.confirm.value }}", "RELEASE_APPROVED_PLAN": "{{ .steps.plan.value }}"}, cut.Steps[2].Env, "dispatch must receive the exact plan shown before confirmation")
	vars := steps.NewVariables()
	// Use the same full template renderer and pass count as native custom commands.
	vars.SetTemplateRenderer(func(name, input string, data any) (string, error) {
		return atmosExec.ProcessStepTmpl(&schema.AtmosConfiguration{}, name, input, data, delimiters.Default())
	})
	vars.SetTemplatePasses(3)
	plan := `{"version":"v1.234.0","commit":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","milestone":{"number":42},"branch":"main","previous_tag":"v1.233.0"}`
	vars.Set("plan", steps.NewStepResult(plan))
	prompt, err := vars.Resolve(cut.Steps[1].Prompt)
	require.NoError(t, err)
	require.Equal(t, "Release 1.234.0\nSource: main @ aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\nPrevious: v1.233.0\n\nDo you want to cut 1.234.0?", prompt)
	for _, answer := range []string{"false", "true"} {
		vars.Set("confirm", steps.NewStepResult(answer))
		env, err := vars.ResolveEnvMap(cut.Steps[2].Env)
		require.NoError(t, err)
		require.Equal(t, answer, env["RELEASE_CONFIRMED"])
		require.Equal(t, plan, env["RELEASE_APPROVED_PLAN"], "native template evaluation must preserve the exact displayed plan")
	}
}

func TestReleaseRehearsalPreservesCoverageAndRunsRealGoReleaser(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "local-action-tests.yml"))
	require.NoError(t, err)
	var workflow struct {
		On map[string]struct {
			Paths []string `yaml:"paths"`
		} `yaml:"on"`
		Jobs map[string]distributionJob `yaml:"jobs"`
	}
	require.NoError(t, yaml.Unmarshal(data, &workflow))
	for _, event := range []string{"pull_request", "push"} {
		require.Contains(t, workflow.On[event].Paths, ".atmos.d/release.yaml", event+" must exercise changes to release entry points")
	}
	rehearsal, ok := workflow.Jobs["release-rehearsal"]
	require.True(t, ok)
	found := false
	for _, step := range rehearsal.Steps {
		if step.Run == "go test -tags=mage -race -count=1 -v -coverprofile=release-coverage.out ./magefiles/release/... ./magefiles/qualitygate ./internal/releaseidentity ./pkg/version" {
			found = true
			require.Equal(t, "goreleaser", step.Env["ATMOS_TEST_GORELEASER"], "coverage reporting must retain real GoReleaser rehearsal")
		}
	}
	require.True(t, found, "CI must collect coverage from all release subpackages with the race detector")
	uploaded := false
	for _, step := range rehearsal.Steps {
		require.False(t, strings.HasPrefix(step.Uses, "codecov/codecov-action@"), "a focused rehearsal must not publish partial project coverage")
		if strings.HasPrefix(step.Uses, "actions/upload-artifact@") && step.With["path"] == "release-coverage.out" {
			uploaded = true
			require.Equal(t, "release-rehearsal-coverage", step.With["name"])
			require.Equal(t, "error", step.With["if-no-files-found"])
		}
	}
	require.True(t, uploaded, "retain the real rehearsal's coverage for diagnosis")
}

// Project coverage must wait for the complete acceptance and tooling reports.
// A fast, narrow suite can otherwise advertise near-perfect project coverage.
func TestRepositoryCoverageWaitsForCompleteSuite(t *testing.T) {
	var workflow struct {
		Jobs map[string]struct {
			Needs any            `yaml:"needs"`
			If    string         `yaml:"if"`
			Steps []pipelineStep `yaml:"steps"`
		} `yaml:"jobs"`
	}
	pipelineYAML(t, ".github/workflows/test.yml", &workflow)
	coverage := workflow.Jobs["coverage"]
	require.ElementsMatch(t, []any{"build", "test", "magefiles"}, coverage.Needs)
	require.Empty(t, coverage.If, "failed prerequisites must prevent a partial upload")
	upload := pipelineUses(t, coverage.Steps, "codecov/codecov-action")
	require.ElementsMatch(t, []string{"coverage.out", "magefiles-coverage.out", "noticegen-coverage.out"}, strings.Split(upload.With["files"], ","))
	require.Equal(t, "true", upload.With["disable_search"], "upload only the explicitly collected reports")
	require.Equal(t, "unittests", upload.With["flags"])
	mage := workflow.Jobs["magefiles"]
	require.GreaterOrEqual(t, pipelineRunIndex(t, mage.Steps, "-coverprofile=magefiles-coverage.out ./magefiles/..."), 0)
}
