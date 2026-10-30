package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

// toolchainLayout is a unit whose HCL runs an executable that exists only in a
// toolchain directory. The ambient process PATH never contains that directory.
type toolchainLayout struct {
	root, unit, module, empty string
	tofu                      string
}

func newToolchainLayout(t *testing.T, hcl string) toolchainLayout {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	layout := toolchainLayout{root: root, unit: filepath.Join(root, "unit"), module: filepath.Join(root, "module"), empty: filepath.Join(root, "empty")}
	for _, dir := range []string{layout.unit, layout.module, layout.empty} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
	}
	// The selected Terraform/OpenTofu executable is always an absolute path outside every PATH.
	layout.tofu = installFixtureExecutable(t, filepath.Join(root, "selected"), toolchainTofuName)
	source, err := json.Marshal(filepath.ToSlash(layout.module))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(layout.unit, "terragrunt.hcl"), []byte("terraform { source = "+string(source)+" }\n"+hcl), 0o644))
	// The ambient PATH has neither the toolchain directory nor the tool.
	t.Setenv("PATH", layout.empty)
	return layout
}

func (l *toolchainLayout) request(environment map[string]string) model.Request {
	return model.Request{Path: filepath.Join(l.unit, "terragrunt.hcl"), BasePath: l.root, Command: "plan", Tofu: l.tofu, Environment: environment}
}

func decodeReport(t *testing.T, value any) (toolchainReport, error) {
	t.Helper()
	text, ok := value.(string)
	if !ok {
		encoded, err := json.Marshal(value)
		if err != nil {
			return toolchainReport{}, err
		}
		text = string(encoded)
	}
	var report toolchainReport
	return report, json.Unmarshal([]byte(text), &report)
}

func mustReport(t *testing.T, value any) toolchainReport {
	t.Helper()
	report, err := decodeReport(t, value)
	require.NoError(t, err)
	return report
}

// A dependencies.tools executable is found through the invocation PATH even though the
// plugin process PATH cannot find it, and the ambient PATH never substitutes for it.
func TestRunCmdFindsToolchainToolAbsentFromAmbientPath(t *testing.T) {
	layout := newToolchainLayout(t, `locals { tool = jsondecode(run_cmd("fixture-tool", "argument")) }
inputs = { tool = local.tool }
`)
	toolchain := filepath.Join(layout.root, "toolchain", "bin")
	tool := installFixtureExecutable(t, toolchain, toolchainToolName)

	t.Run("invocation PATH carries the tool directory", func(t *testing.T) {
		env := map[string]string{"PATH": toolchain + string(os.PathListSeparator) + layout.empty, toolchainChildEnv: "1", "AWS_PROFILE": "development"}
		result, err := Resolve(t.Context(), layout.request(env))
		require.NoError(t, err)
		report := mustReport(t, result.Inputs["tool"])
		require.Equal(t, physicalPath(t, tool), physicalPath(t, report.Executable))
		require.Equal(t, []string{"argument"}, report.Arguments)
		require.Equal(t, env["PATH"], report.Path, "the child must receive the invocation PATH unchanged")
		require.Equal(t, "development", report.Profile)
	})

	t.Run("ambient PATH never substitutes for the invocation PATH", func(t *testing.T) {
		// Now the tool is on the ambient PATH but absent from the invocation PATH.
		t.Setenv("PATH", toolchain)
		env := map[string]string{"PATH": layout.empty, toolchainChildEnv: "1"}
		_, err := Resolve(t.Context(), layout.request(env))
		require.Error(t, err)
		require.ErrorContains(t, err, toolchainToolName)
	})

	t.Run("a missing invocation PATH does not fall back to the ambient PATH", func(t *testing.T) {
		t.Setenv("PATH", toolchain)
		_, err := Resolve(t.Context(), layout.request(map[string]string{toolchainChildEnv: "1"}))
		require.Error(t, err)
		require.ErrorContains(t, err, toolchainToolName)
	})
}

// Two tool versions and two identities evaluated at once in one process never see each other.
func TestToolchainAndIdentityIsolationAcrossConsecutiveAndConcurrentEvaluations(t *testing.T) {
	layout := newToolchainLayout(t, `inputs = {
  tool = jsondecode(run_cmd("fixture-tool"))
  profile = get_env("AWS_PROFILE", "unset")
  custom = get_env("CUSTOM_VALUE", "unset")
  ambient = get_env("ATMOS_TERRAGRUNT_TOOLCHAIN_AMBIENT", "unset")
}
`)
	t.Setenv("ATMOS_TERRAGRUNT_TOOLCHAIN_AMBIENT", "ambient-must-not-return")
	t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "ambient-token-must-not-return")
	versions := map[string]string{}
	for _, version := range []string{"1.0.0", "2.0.0"} {
		versions[version] = installFixtureExecutable(t, filepath.Join(layout.root, "toolchain", version, "bin"), toolchainToolName)
	}
	type scenario struct{ version, profile, custom string }
	scenarios := []scenario{{"1.0.0", "development", "from-stack-dev"}, {"2.0.0", "production", ""}}
	check := func(s scenario) error {
		dir := filepath.Dir(versions[s.version])
		env := map[string]string{"PATH": dir, "AWS_PROFILE": s.profile, "CUSTOM_VALUE": s.custom, toolchainChildEnv: "1"}
		result, err := Resolve(t.Context(), layout.request(env))
		if err != nil {
			return err
		}
		report, err := decodeReport(t, result.Inputs["tool"])
		if err != nil {
			return err
		}
		resolved, err := filepath.EvalSymlinks(report.Executable)
		if err != nil {
			return err
		}
		want, err := filepath.EvalSymlinks(versions[s.version])
		if err != nil {
			return err
		}
		switch {
		case resolved != want:
			return fmt.Errorf("tool %s resolved %s, want %s", s.version, report.Executable, versions[s.version])
		case report.Path != dir:
			return fmt.Errorf("PATH leaked: %q", report.Path)
		case report.Profile != s.profile || result.Inputs["profile"] != s.profile:
			return fmt.Errorf("profile leaked: child %q hcl %v", report.Profile, result.Inputs["profile"])
		case report.Custom != s.custom || result.Inputs["custom"] != s.custom:
			return fmt.Errorf("explicit empty or custom value was replaced: child %q hcl %v", report.Custom, result.Inputs["custom"])
		case report.Token != "" || report.Ambient != "":
			return fmt.Errorf("ambient environment returned: %+v", report)
		case result.Inputs["ambient"] != "unset":
			return fmt.Errorf("ambient value visible to get_env: %v", result.Inputs["ambient"])
		}
		return nil
	}
	for _, s := range []scenario{scenarios[0], scenarios[1], scenarios[1], scenarios[0]} {
		require.NoError(t, check(s), "consecutive")
	}
	var workers sync.WaitGroup
	failures := make(chan error, 32)
	for i := range 32 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if err := check(scenarios[i%2]); err != nil {
				failures <- err
			}
		}()
	}
	workers.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err, "concurrent")
	}
	require.Equal(t, "ambient-must-not-return", os.Getenv("ATMOS_TERRAGRUNT_TOOLCHAIN_AMBIENT"))
}

// Explicit relative commands and working directories resolve from the unit, not the plugin process.
func TestRunCmdRelativePathsAndWorkingDirectory(t *testing.T) {
	// Explicit paths carry the platform's executable suffix; bare names are resolved through PATH.
	suffix := executableSuffix()
	layout := newToolchainLayout(t, `locals {
  here     = jsondecode(run_cmd("./tools/fixture-tool`+suffix+`"))
  sibling  = jsondecode(run_cmd("../shared/fixture-tool`+suffix+`"))
  absolute = jsondecode(run_cmd("${get_terragrunt_dir()}/tools/fixture-tool`+suffix+`"))
}
inputs = { here = local.here, sibling = local.sibling, absolute = local.absolute, dir = get_terragrunt_dir() }
`)
	here := installFixtureExecutable(t, filepath.Join(layout.unit, "tools"), toolchainToolName)
	sibling := installFixtureExecutable(t, filepath.Join(layout.root, "shared"), toolchainToolName)
	// The process working directory differs from the unit; a relative command must not use it.
	t.Chdir(layout.empty)
	result, err := Resolve(t.Context(), layout.request(map[string]string{"PATH": layout.empty, toolchainChildEnv: "1"}))
	require.NoError(t, err)
	for key, want := range map[string]string{"here": here, "sibling": sibling, "absolute": here} {
		report := mustReport(t, result.Inputs[key])
		require.Equal(t, physicalPath(t, want), physicalPath(t, report.Executable), key)
		require.Equal(t, physicalPath(t, layout.unit), physicalPath(t, report.Directory), "%s must run in the unit directory", key)
	}
	require.Equal(t, filepath.ToSlash(physicalPath(t, layout.unit)), result.Inputs["dir"])
}

// A parent configuration included by the unit evaluates commands in the unit directory.
func TestIncludedConfigurationRunsCommandsFromUnitDirectory(t *testing.T) {
	layout := newToolchainLayout(t, `include "root" {
  path   = find_in_parent_folders("root.hcl")
  expose = true
}
inputs = { tool = include.root.locals.tool }
`)
	require.NoError(t, os.WriteFile(filepath.Join(layout.root, "root.hcl"), []byte(`locals { tool = jsondecode(run_cmd("fixture-tool")) }
`), 0o644))
	toolchain := filepath.Join(layout.root, "toolchain")
	tool := installFixtureExecutable(t, toolchain, toolchainToolName)
	result, err := Resolve(t.Context(), layout.request(map[string]string{"PATH": toolchain, toolchainChildEnv: "1"}))
	require.NoError(t, err)
	report := mustReport(t, result.Inputs["tool"])
	require.Equal(t, physicalPath(t, tool), physicalPath(t, report.Executable))
	require.Equal(t, physicalPath(t, layout.unit), physicalPath(t, report.Directory))
}
