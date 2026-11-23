package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	// get_terragrunt_dir() returns native separators, which are backslashes on Windows.
	dir, isString := result.Inputs["dir"].(string)
	require.True(t, isString)
	require.Equal(t, physicalPath(t, layout.unit), filepath.Clean(filepath.FromSlash(dir)))
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

// toolchainChildEnv gates the re-executed test binary into a fake toolchain executable.
// The child is a "tofu" when its file name starts with toolchainTofuPrefix and an
// ordinary HCL tool otherwise, so one environment can reach both kinds of child.
const (
	toolchainChildEnv = "ATMOS_TERRAGRUNT_TOOLCHAIN_CHILD"
	toolchainCallLog  = "ATMOS_TERRAGRUNT_TOOLCHAIN_CALL_LOG"
	toolchainTofuName = "fixture-tofu"
	toolchainToolName = "fixture-tool"
)

// toolchainReport is what every fake executable observed about its own invocation.
type toolchainReport struct {
	Executable string            `json:"executable"`
	Directory  string            `json:"directory"`
	Arguments  []string          `json:"arguments"`
	Path       string            `json:"path"`
	Profile    string            `json:"profile"`
	Custom     string            `json:"custom"`
	Token      string            `json:"token"`
	Ambient    string            `json:"ambient"`
	Environ    map[string]string `json:"environ,omitempty"`
}

func currentToolchainReport() toolchainReport {
	executable, _ := os.Executable()
	directory, _ := os.Getwd()
	return toolchainReport{
		Executable: executable, Directory: directory, Arguments: os.Args[1:], Path: os.Getenv("PATH"),
		Profile: os.Getenv("AWS_PROFILE"), Custom: os.Getenv("CUSTOM_VALUE"), Token: os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE"),
		Ambient: os.Getenv("ATMOS_TERRAGRUNT_TOOLCHAIN_AMBIENT"),
	}
}

// runToolchainChild implements the fake executables. It reports whether this process is one.
func runToolchainChild() bool {
	if os.Getenv(toolchainChildEnv) != "1" {
		return false
	}
	report := currentToolchainReport()
	if path := os.Getenv(toolchainCallLog); path != "" {
		if file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
			_ = json.NewEncoder(file).Encode(report)
			_ = file.Close()
		}
	}
	if !strings.HasPrefix(filepath.Base(os.Args[0]), toolchainTofuName) {
		_ = json.NewEncoder(os.Stdout).Encode(report)
		return true
	}
	if len(os.Args) > 1 && os.Args[1] == "output" {
		encoded, _ := json.Marshal(report)
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"report": map[string]any{"value": string(encoded), "type": "string", "sensitive": false}})
	}
	if len(os.Args) > 1 && os.Args[1] == "version" {
		_, _ = os.Stdout.WriteString(`{"terraform_version":"1.9.0"}`)
	}
	return true
}

// installFixtureExecutable places a hard link (or copy) of the test binary at dir/name.
func installFixtureExecutable(t *testing.T, dir, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	require.NoError(t, os.MkdirAll(dir, 0o755))
	target := filepath.Join(dir, name)
	executable, err := os.Executable()
	require.NoError(t, err)
	if err := os.Link(executable, target); err == nil {
		return target
	}
	content, err := os.ReadFile(executable)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(target, content, 0o755))
	return target
}

// physicalPath removes platform temp-directory symlinks so equal directories compare equal.
func physicalPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	return filepath.Clean(resolved)
}

// executableSuffix is the file name suffix of an executable on this platform.
func executableSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
