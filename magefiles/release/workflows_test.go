package release

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/automation"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/script"
	starlarkengine "github.com/cloudposse/atmos/pkg/script/starlark"
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

type releaseStepLibrary struct {
	run func(*automation.StepCall) (*automation.StepResult, error)
}

func (s *releaseStepLibrary) Names() []string              { return []string{"shell", "confirm"} }
func (s *releaseStepLibrary) Fork() automation.StepLibrary { return s }
func (s *releaseStepLibrary) Validate(call *automation.StepCall) error {
	return step.NewAutomationLibrary(nil, nil).Validate(call)
}

func (s *releaseStepLibrary) Run(_ context.Context, call *automation.StepCall) (*automation.StepResult, error) {
	if err := s.Validate(call); err != nil {
		return nil, err
	}
	return s.run(call)
}

type releasePlanRunner func(process.TaskSpec) process.Result

//nolint:gocritic // process.Runner requires TaskSpec by value.
func (r releasePlanRunner) Run(_ context.Context, spec process.TaskSpec) process.Result {
	return r(spec)
}

func TestNativeReleaseCutConfirmsDisplayedPlanBeforeDispatch(t *testing.T) {
	var config struct {
		Commands []struct {
			Name     string `yaml:"name"`
			Commands []struct {
				Name  string `yaml:"name"`
				Flags []struct {
					Name    string `yaml:"name"`
					Type    string `yaml:"type"`
					Default bool   `yaml:"default"`
				} `yaml:"flags"`
				Steps []struct {
					Type        string `yaml:"type"`
					Interpreter string `yaml:"interpreter"`
					Script      string `yaml:"script"`
				} `yaml:"steps"`
			} `yaml:"commands"`
		} `yaml:"commands"`
	}
	data, err := os.ReadFile(filepath.Join("..", "..", ".atmos.d", "release.yaml"))
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(data, &config))
	source := ""
	for _, root := range config.Commands {
		for _, command := range root.Commands {
			if root.Name != "release" || command.Name != "cut" {
				continue
			}
			require.Len(t, command.Flags, 1)
			require.Equal(t, "resume", command.Flags[0].Name)
			require.Equal(t, "bool", command.Flags[0].Type)
			require.False(t, command.Flags[0].Default)
			require.Len(t, command.Steps, 1)
			require.Equal(t, "script", command.Steps[0].Type)
			require.Equal(t, "starlark", command.Steps[0].Interpreter)
			source = command.Steps[0].Script
		}
	}
	require.NotEmpty(t, source)
	for _, resume := range []bool{false, true} {
		for _, answer := range []string{"true", "false", "", "plan failed"} {
			t.Run(fmt.Sprintf("resume=%t/answer=%s", resume, answer), func(t *testing.T) {
				plan := `{"version":"v1.234.0","commit":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","milestone":{"number":42},"branch":"main","previous_tag":"v1.233.0","resume":{"run_id":77}}`
				calls := 0
				library := &releaseStepLibrary{run: func(call *automation.StepCall) (*automation.StepResult, error) {
					calls++
					switch calls {
					case 2:
						require.Equal(t, "confirm", call.Type)
						require.Equal(t, false, call.Configuration["default"])
						prompt := call.Configuration["prompt"].(string)
						require.Contains(t, prompt, "Source: main @ aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
						if resume {
							require.Contains(t, prompt, "Do you want to resume 1.234.0?")
							require.Contains(t, prompt, "https://github.com/cloudposse/atmos/actions/runs/77")
						} else {
							require.Contains(t, prompt, "Do you want to cut 1.234.0?")
						}
						return &automation.StepResult{Value: answer}, nil
					case 3:
						require.Equal(t, "true", answer, "declining must never execute the release command")
						require.Equal(t, "shell", call.Type)
						require.Equal(t, true, call.Configuration["interactive"])
						require.Equal(t, "go tool mage release:cut cloudposse/atmos", call.Configuration["command"])
						require.Equal(t, map[string]any{"RELEASE_RESUME": fmt.Sprint(resume), "RELEASE_CONFIRMED": "true", "RELEASE_APPROVED_PLAN": plan}, call.Configuration["env"])
						return &automation.StepResult{}, nil
					default:
						t.Fatal("unexpected step")
						return nil, nil
					}
				}}
				runner := releasePlanRunner(func(spec process.TaskSpec) process.Result {
					calls++
					require.Equal(t, 1, calls, "planning must precede confirmation")
					require.Equal(t, "go", spec.Command)
					require.Equal(t, []string{"tool", "mage", "release:planJSON", "cloudposse/atmos"}, spec.Args)
					require.Contains(t, spec.Env, "RELEASE_RESUME="+fmt.Sprint(resume))
					if answer == "plan failed" {
						return process.Result{ExitCode: 1}
					}
					_, err := io.WriteString(spec.Streams.Stdout, plan)
					require.NoError(t, err)
					return process.Result{}
				})
				_, err := starlarkengine.New(starlarkengine.WithProcessRunner(runner)).Execute(t.Context(), script.Spec{Source: source, Flags: map[string]any{"resume": resume}, Steps: library})
				if answer == "true" {
					require.NoError(t, err)
					require.Equal(t, 3, calls)
				} else if answer == "plan failed" {
					require.Error(t, err)
					require.Equal(t, 1, calls, "failed planning must not confirm or dispatch")
				} else {
					require.ErrorContains(t, err, "Release cancelled")
					require.Equal(t, 2, calls)
				}
			})
		}
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
