package terraform

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPlanDiffInitPreservesCallerPlan(t *testing.T) {
	root := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: root}
	config.Components.Terraform.BasePath = "components"
	dir := filepath.Join(root, "components", "probe")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	info := schema.ConfigAndStacksInfo{Component: "probe", FinalComponent: "probe", ContextPrefix: "dev", SubCommand: "plan-diff"}
	plan := filepath.Join(dir, "dev-probe.planfile")
	require.NoError(t, os.WriteFile(plan, []byte("reviewed plan"), 0o600))
	host := NewMockOperationServices(gomock.NewController(t))
	host.EXPECT().ExecuteTerraform(gomock.Any()).DoAndReturn(func(actual schema.ConfigAndStacksInfo, _ ...component.ExecutionOption) error {
		require.Equal(t, "init", actual.SubCommand)
		cleanupTerraformFiles(config, &actual)
		return nil
	})
	require.NoError(t, NewOperations(host).runTerraformInit(config, dir, &info))
	data, err := os.ReadFile(plan)
	require.NoError(t, err)
	require.Equal(t, "reviewed plan", string(data))
	require.Empty(t, info.PlanFile, "do not mutate caller command metadata")
}

func TestPlanDiffTemporaryCopyDoesNotOwnCallerFiles(t *testing.T) {
	for _, relative := range []bool{false, true} {
		t.Run(map[bool]string{true: "relative", false: "absolute"}[relative], func(t *testing.T) {
			root := t.TempDir()
			dir, sibling := filepath.Join(root, "component"), filepath.Join(root, "component-sibling")
			require.NoError(t, os.MkdirAll(dir, 0o755))
			require.NoError(t, os.MkdirAll(sibling, 0o755))
			original, collision := filepath.Join(sibling, "reviewed.plan"), filepath.Join(dir, "reviewed.plan")
			require.NoError(t, os.WriteFile(original, []byte("source plan"), 0o600))
			require.NoError(t, os.WriteFile(collision, []byte("other reviewed plan"), 0o600))
			source, target := original, dir
			if relative {
				t.Chdir(root)
				source, target = filepath.Join("component-sibling", "reviewed.plan"), "component"
			}
			copied, cleanup, err := CopyPlanFileIfNeeded(source, target)
			require.NoError(t, err)
			require.NotNil(t, cleanup)
			require.NotEqual(t, original, copied)
			require.NotEqual(t, collision, copied)
			bytes, err := os.ReadFile(copied)
			require.NoError(t, err)
			require.Equal(t, "source plan", string(bytes))
			cleanup()
			require.NoFileExists(t, copied)
			for file, want := range map[string]string{original: "source plan", collision: "other reviewed plan"} {
				bytes, err := os.ReadFile(file)
				require.NoError(t, err)
				require.Equal(t, want, string(bytes))
			}
			same, cleanup, err := CopyPlanFileIfNeeded(collision, dir)
			require.NoError(t, err)
			require.Equal(t, collision, same)
			require.Nil(t, cleanup)
		})
	}
}

func TestPlanDiffComparisonReturnsStatusAndCleansOwnedCopies(t *testing.T) {
	for _, different := range []bool{false, true} {
		t.Run(map[bool]string{true: "different", false: "identical"}[different], func(t *testing.T) {
			directory, sources := t.TempDir(), t.TempDir()
			original, fresh := filepath.Join(sources, "original.plan"), filepath.Join(sources, "fresh.plan")
			oldJSON := `{"variables":{"revision":{"value":"one"}}}`
			newJSON := oldJSON
			if different {
				newJSON = `{"variables":{"revision":{"value":"two"}}}`
			}
			require.NoError(t, os.WriteFile(original, []byte(oldJSON), 0o600))
			require.NoError(t, os.WriteFile(fresh, []byte(newJSON), 0o600))
			host := NewMockOperationServices(gomock.NewController(t))
			host.EXPECT().ExecuteTerraform(gomock.Any(), gomock.Any()).DoAndReturn(func(actual schema.ConfigAndStacksInfo, options ...component.ExecutionOption) error {
				require.Equal(t, "show", actual.SubCommand)
				require.Equal(t, "-json", actual.AdditionalArgsAndFlags[0])
				content, err := os.ReadFile(actual.AdditionalArgsAndFlags[1])
				require.NoError(t, err)
				var execution component.ExecutionOptions
				for _, option := range options {
					option(&execution)
				}
				_, err = execution.StdoutOverride.Write(content)
				return err
			}).Times(2)
			err := NewOperations(host).ComparePlansAndGenerateDiff(&schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{SkipInit: true}, directory, original, fresh)
			if different {
				require.ErrorIs(t, err, errUtils.ErrPlanHasDiff)
				require.Equal(t, 2, errUtils.GetExitCode(err))
			} else {
				require.NoError(t, err)
			}
			for file, expected := range map[string]string{original: oldJSON, fresh: newJSON} {
				actual, err := os.ReadFile(file)
				require.NoError(t, err)
				require.Equal(t, expected, string(actual))
			}
			files, err := os.ReadDir(directory)
			require.NoError(t, err)
			require.Empty(t, files, "only owned temporary copies may be removed")
		})
	}
}

// Copy failures must not leave a partial plan available for a later deployment.
func TestPlanDiffCopyFailurePreservesCallerFiles(t *testing.T) {
	for _, scenario := range []string{"missing source", "unreadable source contents", "unavailable destination"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			directory := filepath.Join(root, "component")
			require.NoError(t, os.Mkdir(directory, 0o755))
			callerPlan := filepath.Join(directory, "reviewed.plan")
			require.NoError(t, os.WriteFile(callerPlan, []byte("approved caller plan"), 0o600))
			source := filepath.Join(root, "reviewed.plan")
			destination := directory
			switch scenario {
			case "missing source":
				// Leave the external plan absent.
			case "unreadable source contents":
				// A directory can open on Unix, but reading it as plan bytes fails.
				require.NoError(t, os.Mkdir(source, 0o755))
			case "unavailable destination":
				require.NoError(t, os.WriteFile(source, []byte("source plan"), 0o600))
				destination = filepath.Join(root, "missing-component")
			}

			copied, cleanup, err := CopyPlanFileIfNeeded(source, destination)
			require.Error(t, err)
			require.Empty(t, copied)
			require.Nil(t, cleanup, "failure must clean up internally")
			content, err := os.ReadFile(callerPlan)
			require.NoError(t, err)
			require.Equal(t, "approved caller plan", string(content))
			files, err := os.ReadDir(directory)
			require.NoError(t, err)
			require.Len(t, files, 1, "no partial temporary plan may survive")
			require.Equal(t, "reviewed.plan", files[0].Name())
			if scenario == "unavailable destination" {
				content, err = os.ReadFile(source)
				require.NoError(t, err)
				require.Equal(t, "source plan", string(content))
			}
		})
	}
}
