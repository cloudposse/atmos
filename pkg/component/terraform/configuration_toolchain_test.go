package terraform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/dependencies"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/component"
	configadapter "github.com/cloudposse/atmos/pkg/plugin/adapter/configuration"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

// testEnvToolchainProbe names a file. A process started with it set records how it was
// invoked into that file and exits, which lets a test observe the executable, directory,
// arguments, and environment of every Terraform subprocess without any Unix-only binary.
const testEnvToolchainProbe = "_ATMOS_TEST_TOOLCHAIN_PROBE"

// testEnvProbeExit makes a toolchain probe exit with that code after it recorded a command other than --version.
const testEnvProbeExit = "_ATMOS_TEST_PROBE_EXIT"

// toolchainProbe is one recorded subprocess invocation.
type toolchainProbe struct {
	Executable string   `json:"executable"`
	Directory  string   `json:"directory"`
	Arguments  []string `json:"arguments"`
	Path       string   `json:"path"`
	Profile    string   `json:"profile"`
	Custom     string   `json:"custom"`
	Token      string   `json:"token"`
	Ambient    string   `json:"ambient"`
	Identity   string   `json:"identity"`
	// Terragrunt executable mode: the TG_* variables Atmos sets for the unit.
	NonInteractive string `json:"non_interactive"`
	TFPath         string `json:"tf_path"`
	WebIdentity    string `json:"web_identity"`
}

func runToolchainProbeForTest(path string) int {
	executable, _ := os.Executable()
	directory, _ := os.Getwd()
	probe := toolchainProbe{
		Executable: executable, Directory: directory, Arguments: os.Args[1:], Path: os.Getenv("PATH"),
		Profile: os.Getenv("AWS_PROFILE"), Custom: os.Getenv("CUSTOM_VALUE"), Token: os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE"),
		Ambient: os.Getenv("ATMOS_TOOLCHAIN_AMBIENT_ONLY"), Identity: os.Getenv("ATMOS_IDENTITY"),
		NonInteractive: os.Getenv("TG_NON_INTERACTIVE"), TFPath: os.Getenv("TG_TF_PATH"), WebIdentity: os.Getenv("TG_IAM_ASSUME_ROLE_WEB_IDENTITY_TOKEN"),
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return 1
	}
	defer file.Close()
	if err := json.NewEncoder(file).Encode(probe); err != nil {
		return 1
	}
	// Acting as terragrunt, the probe reports a version and can be told to fail the command.
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println("terragrunt version v1.2.1")
		return 0
	}
	if code, err := strconv.Atoi(os.Getenv(testEnvProbeExit)); err == nil {
		return code
	}
	return 0
}

func readToolchainProbes(t *testing.T, path string) []toolchainProbe {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	var probes []toolchainProbe
	decoder := json.NewDecoder(bytes.NewReader(data))
	for decoder.More() {
		var probe toolchainProbe
		require.NoError(t, decoder.Decode(&probe))
		probes = append(probes, probe)
	}
	return probes
}

// When the process PATH already starts with the toolchain directories (for example an Atmos run
// nested inside a toolchain shell), replacing PATH still preserves the toolchain directories.
func TestToolchainPathEnvWithToolchainDirectoriesAlreadyInAmbientPath(t *testing.T) {
	harness := newToolchainHarness(t, "1.0.0")
	info := harness.info(t, &toolchainScenario{identity: "development", tofuVersion: "1.0.0", toolVersion: "1.0.0"}, filepath.Join(t.TempDir(), "probe.jsonl"))

	tenv, err := resolveAndInstallToolchainDeps(harness.config, info)
	require.NoError(t, err)
	full := tenv.PATH()
	require.NotEmpty(t, full)
	t.Setenv("PATH", full)

	selected := filepath.Join(t.TempDir(), "selected")
	got := toolchainPathEnv(&schema.ConfigAndStacksInfo{ComponentEnvList: []string{"PATH=" + selected}}, tenv)
	require.Equal(t, []string{"PATH=" + tenv.PrependToPath(selected)}, got)

	// An empty selected PATH retains only the toolchain directories.
	got = toolchainPathEnv(&schema.ConfigAndStacksInfo{ComponentEnvList: []string{"PATH="}}, tenv)
	require.Equal(t, []string{"PATH=" + tenv.PrependToPath("")}, got)
	require.NotEmpty(t, tenv.ToolchainDirs())
}

// installToolchainFixture places a hard link (or copy) of the test binary where the real
// toolchain installer would have installed owner/repo at version, so no network is used.
func installToolchainFixture(t *testing.T, installPath, owner, repo, version, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	target := filepath.Join(installPath, "bin", owner, repo, version, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
	executable, err := os.Executable()
	require.NoError(t, err)
	// Windows refuses to delete a hard link to the running test binary, which breaks TempDir cleanup, so copy there.
	if runtime.GOOS != "windows" && os.Link(executable, target) == nil {
		return target
	}
	content, err := os.ReadFile(executable)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(target, content, 0o755))
	return target
}

func physical(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	return filepath.Clean(resolved)
}

// pathContains reports whether the fixture tool is an executable file in one of the PATH directories.
func pathContains(path string) (string, bool) {
	name := "fixture-tool"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	for _, dir := range filepath.SplitList(path) {
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
	}
	return "", false
}

// recordingConfigurationProvider stores observations from generated mock callbacks.
type recordingConfigurationProvider struct {
	mu       sync.Mutex
	events   *[]string
	requests []*pluginv1.ComponentRequest
}

func newRecordingConfigurationProvider(t *testing.T, events *[]string) (*MockConfigurationProvider, *recordingConfigurationProvider) {
	t.Helper()
	p := &recordingConfigurationProvider{events: events}
	provider := NewMockConfigurationProvider(gomock.NewController(t))
	provider.EXPECT().ComponentType().Return("terraform").AnyTimes()
	provider.EXPECT().Discover(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, *pluginv1.DiscoverComponentsRequest) (*pluginv1.DiscoverComponentsResponse, error) {
		configuration, err := adapter.EncodeObject(map[string]any{"source": "./module", "vars": map[string]any{}})
		if err != nil {
			return nil, err
		}
		return &pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{{
			Address:  &pluginv1.ComponentAddress{Stack: "live", Component: "app", ComponentType: "terraform"},
			Provider: "terragrunt", Engine: "opentofu", Source: "live/app/terragrunt.hcl", Configuration: configuration,
		}}}, nil
	}).AnyTimes()
	backend, err := adapter.EncodeObject(map[string]any{"path": "terraform.tfstate"})
	require.NoError(t, err)
	inputs, err := adapter.EncodeObject(map[string]any{})
	require.NoError(t, err)
	provider.EXPECT().Prepare(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, request *pluginv1.ComponentRequest) (*pluginv1.PrepareComponentResponse, error) {
		p.mu.Lock()
		defer p.mu.Unlock()
		*p.events = append(*p.events, "evaluate")
		p.requests = append(p.requests, request)
		return &pluginv1.PrepareComponentResponse{Engine: "opentofu", ModuleSource: "./module", Inputs: inputs, State: &pluginv1.ComponentStateIdentity{BackendType: "local", Backend: backend, Workspace: "default"}}, nil
	}).AnyTimes()
	provider.EXPECT().ResolveState(gomock.Any(), gomock.Any()).Return(&pluginv1.ResolveComponentStateResponse{State: &pluginv1.ComponentStateIdentity{BackendType: "local", Backend: backend, Workspace: "default"}}, nil).AnyTimes()
	return provider, p
}

// toolchainScenario is one component execution: an identity and the tool versions it pins.
type toolchainScenario struct {
	identity, tofuVersion, toolVersion, stackValue string
	subCommand                                     string // Defaults to plan.
	stackPath                                      string // Optional PATH set in the stack env section.
}

// toolchainHarness wires the real authentication hook, the real toolchain resolver over a
// pre-seeded install directory, the real plugin adapter session, and the real command pipeline.
type toolchainHarness struct {
	config    *schema.AtmosConfiguration
	executor  *Executor
	provider  *recordingConfigurationProvider
	events    []string
	tools     map[string]string // "name@version" -> installed path.
	ambient   string
	afterAuth func(*schema.ConfigAndStacksInfo) // Optional identity-environment adjustment.
}

func newToolchainHarness(t *testing.T, versions ...string) *toolchainHarness {
	t.Helper()
	previous := toolchain.GetAtmosConfig()
	t.Cleanup(func() { toolchain.SetAtmosConfig(previous) })

	// Credentials and config written by the mock identity stay inside the test.
	home := t.TempDir()
	for _, key := range []string{"HOME", "USERPROFILE", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(key, home)
	}
	// The ambient PATH has neither the toolchain directories nor either tool.
	ambient := t.TempDir()
	t.Setenv("PATH", ambient)
	t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "ambient-token-must-not-return")
	t.Setenv("ATMOS_TOOLCHAIN_AMBIENT_ONLY", "ambient")
	t.Setenv("CUSTOM_VALUE", "ambient")

	root := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: root, BasePathAbsolute: root}
	config.Toolchain = schema.Toolchain{
		Install: schema.ToolchainInstallNever, InstallPath: filepath.Join(root, "tools"),
		Aliases: map[string]string{"tofu": "opentofu/opentofu", "fixture-tool": "acme/fixture-tool"},
	}
	config.Components.Terraform.Command = "tofu"
	config.Components.Terraform.ConfigurationProviders = map[string]schema.ComponentConfiguration{
		"terragrunt": {Enabled: true, Discovery: schema.ComponentDiscoveryConfig{Enabled: true, IncludedPaths: []string{"live/**"}}},
	}
	harness := &toolchainHarness{config: config, executor: testExecutor(t), tools: map[string]string{}, ambient: ambient}
	for _, version := range versions {
		harness.tools["tofu@"+version] = installToolchainFixture(t, config.Toolchain.InstallPath, "opentofu", "opentofu", version, "tofu")
		harness.tools["fixture-tool@"+version] = installToolchainFixture(t, config.Toolchain.InstallPath, "acme", "fixture-tool", version, "fixture-tool")
	}

	provider, recorder := newRecordingConfigurationProvider(t, &harness.events)
	harness.provider = recorder
	session, err := configadapter.New(t.Context(), config, "plan", func(context.Context, []string) (map[string]configadapter.Provider, error) {
		return map[string]configadapter.Provider{"terragrunt": provider}, nil
	})
	require.NoError(t, err)
	config.ComponentConfigurations = session

	// Observe the real auth hook and the real toolchain resolver without replacing them.
	authenticate, resolve := harness.executor.prepareAuthEnvironmentFn, harness.executor.resolveConfigurationToolsFn
	harness.executor.prepareAuthEnvironmentFn = func(c *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
		harness.events = append(harness.events, "authenticate")
		if err := authenticate(c, info); err != nil {
			return err
		}
		// Real cloud identities remove ambient credential variables from the sanitized
		// environment; the mock identity does not. Do the same so the test can prove the
		// host never reintroduces them downstream.
		sanitized := info.SanitizedEnv[:0:0]
		for _, entry := range info.SanitizedEnv {
			if !strings.HasPrefix(entry, "AWS_WEB_IDENTITY_TOKEN_FILE=") {
				sanitized = append(sanitized, entry)
			}
		}
		info.SanitizedEnv = sanitized
		if harness.afterAuth != nil {
			harness.afterAuth(info)
		}
		return nil
	}
	harness.executor.resolveConfigurationToolsFn = func(c *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*dependencies.ToolchainEnvironment, error) {
		harness.events = append(harness.events, "toolchain")
		require.NotEmpty(t, info.SanitizedEnv, "the identity environment must exist before the toolchain is resolved")
		return resolve(c, info)
	}
	return harness
}

func (h *toolchainHarness) info(t *testing.T, s *toolchainScenario, probe string) *schema.ConfigAndStacksInfo {
	t.Helper()
	identities := map[string]any{}
	for _, name := range []string{"development", "production"} {
		identities[name] = map[string]any{"kind": "mock/aws", "default": name == "development", "via": map[string]any{"provider": "mock-provider"}, "principal": map[string]any{"account": map[string]any{"id": "123456789012"}}}
	}
	subCommand := s.subCommand
	if subCommand == "" {
		subCommand = "plan"
	}
	env := map[string]any{"CUSTOM_VALUE": s.stackValue, testEnvToolchainProbe: probe}
	if s.stackPath != "" {
		env["PATH"] = s.stackPath
	}
	return &schema.ConfigAndStacksInfo{
		Stack: "live", ComponentFromArg: "app", Command: "tofu", SubCommand: subCommand, Identity: s.identity,
		AdditionalArgsAndFlags: autoApproveFor(subCommand),
		ComponentAuthSection: map[string]any{
			"providers":  map[string]any{"mock-provider": map[string]any{"kind": "mock/aws"}},
			"identities": identities,
		},
		ComponentSettingsSection: map[string]any{"configuration": map[string]any{schema.ConfigurationOwnedKey: true}},
		ComponentSection: map[string]any{"dependencies": map[string]any{"tools": map[string]any{
			"tofu": s.tofuVersion, "fixture-tool": s.toolVersion,
		}}},
		ComponentEnvSection: env,
	}
}

func autoApproveFor(subCommand string) []string {
	if subCommand == "apply" || subCommand == "destroy" {
		return []string{"-auto-approve"}
	}
	return nil
}

// run performs the host's ordered preparation for one component and then its Terraform pipeline.
func (h *toolchainHarness) run(t *testing.T, s *toolchainScenario) (*schema.ConfigAndStacksInfo, []toolchainProbe, *pluginv1.ComponentRequest) {
	t.Helper()
	probePath := filepath.Join(t.TempDir(), "probe.jsonl")
	info := h.info(t, s, probePath)
	h.events = nil
	h.provider.requests = nil

	_, err := h.executor.prepareConfigurationEnvironment(t.Context(), h.config, info, true)
	require.NoError(t, err)
	prepared, err := prepareConfiguration(t.Context(), h.config, info)
	require.NoError(t, err)
	require.NotNil(t, prepared)
	require.Len(t, h.provider.requests, 1)

	// Module vendoring is not under test; without a source the init provisioners have nothing to fetch.
	delete(info.ComponentSection, "source")

	// Implicit initialization, workspace selection, and the operation all run in the pipeline.
	origStdin := os.Stdin
	os.Stdin = nil
	t.Cleanup(func() { os.Stdin = origStdin })
	dir := t.TempDir()
	execCtx := &componentExecContext{componentPath: dir, workingDir: dir, varFile: "app.terraform.tfvars.json", planFile: "app.planfile"}
	// execute() hands the identity's sanitized environment to the pipeline exactly like this.
	require.NoError(t, h.executor.executeCommandPipeline(h.config, info, execCtx, WithEnvironment(info.SanitizedEnv)))
	return info, readToolchainProbes(t, probePath), h.provider.requests[0]
}

// Authentication, then toolchain resolution, then evaluation. Every Terraform subprocess
// (implicit init, workspace selection, and the operation itself) runs the pinned executable
// with the identity's environment, and a dependencies.tools executable is reachable on the
// PATH that crosses the plugin boundary even though it is absent from the ambient PATH.
func TestConfigurationToolchainEndToEndThroughAuthPluginAndPipeline(t *testing.T) {
	harness := newToolchainHarness(t, "1.0.0", "2.0.0")
	scenario := toolchainScenario{identity: "production", tofuVersion: "1.0.0", toolVersion: "1.0.0", stackValue: "from-stack"}

	info, probes, request := harness.run(t, &scenario)

	require.Equal(t, []string{"authenticate", "toolchain", "evaluate"}, harness.events)

	pinned := harness.tools["tofu@1.0.0"]
	tool := harness.tools["fixture-tool@1.0.0"]
	require.Equal(t, pinned, info.Command, "info.Command must be the pinned toolchain executable")
	require.True(t, filepath.IsAbs(info.Command))

	environment := request.Invocation.Environment
	require.Equal(t, pinned, environment["ATMOS_TERRAFORM_BINARY"])
	require.Equal(t, "production", environment["AWS_PROFILE"], "identity-provided environment crosses the boundary")
	require.Equal(t, "production", environment["ATMOS_IDENTITY"])
	require.Equal(t, "from-stack", environment["CUSTOM_VALUE"], "stack env beats ambient env")
	require.NotContains(t, environment, "AWS_WEB_IDENTITY_TOKEN_FILE", "ambient credentials must not return behind an identity")

	wirePath := environment["PATH"]
	found, ok := pathContains(wirePath)
	require.True(t, ok, "dependencies.tools executables must be on the PATH sent to the plugin: %s", wirePath)
	require.Equal(t, physical(t, tool), physical(t, found))
	_, ambientHasTool := pathContains(os.Getenv("PATH"))
	require.False(t, ambientHasTool, "the tool must be absent from the ambient PATH for this test to mean anything")
	// The two toolchain directories precede the ambient PATH (their relative order is unspecified).
	entries := filepath.SplitList(wirePath)
	require.Len(t, entries, 3)
	require.ElementsMatch(t, []string{filepath.Dir(pinned), filepath.Dir(tool)}, entries[:2], "toolchain directories precede everything else")
	require.Equal(t, harness.ambient, entries[2])

	// Every spawned subprocess is the pinned executable, with the identity and toolchain PATH.
	require.NotEmpty(t, probes)
	commands := []string{}
	for _, probe := range probes {
		require.Equal(t, physical(t, pinned), physical(t, probe.Executable), "args %v", probe.Arguments)
		require.Equal(t, wirePath, probe.Path, "args %v", probe.Arguments)
		require.Equal(t, "production", probe.Profile)
		require.Equal(t, "from-stack", probe.Custom)
		require.Empty(t, probe.Token, "ambient credentials must not reach Terraform")
		commands = append(commands, strings.Join(probe.Arguments, " "))
	}
	require.True(t, containsPrefix(commands, "init"), "implicit init must run the pinned executable: %v", commands)
	require.True(t, containsPrefix(commands, "plan"), "the operation must run the pinned executable: %v", commands)
}

func containsPrefix(values []string, prefix string) bool {
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

// Two components with different identities and different pinned tool versions run one after the
// other and at the same time in one process. Neither sees the other's PATH, tools, or identity.
func TestConfigurationToolchainIsolatesIdentitiesAndVersions(t *testing.T) {
	harness := newToolchainHarness(t, "1.0.0", "2.0.0")
	scenarios := []toolchainScenario{
		{identity: "development", tofuVersion: "1.0.0", toolVersion: "1.0.0", stackValue: "dev-value"},
		{identity: "production", tofuVersion: "2.0.0", toolVersion: "2.0.0", stackValue: ""},
	}
	check := func(t *testing.T, s toolchainScenario) {
		t.Helper()
		info, probes, request := harness.run(t, &s)
		pinned := harness.tools["tofu@"+s.tofuVersion]
		require.Equal(t, pinned, info.Command)
		environment := request.Invocation.Environment
		require.Equal(t, s.identity, environment["AWS_PROFILE"])
		require.Equal(t, s.stackValue, environment["CUSTOM_VALUE"], "an explicitly empty stack value must not fall back to the ambient value")
		found, ok := pathContains(environment["PATH"])
		require.True(t, ok)
		require.Equal(t, physical(t, harness.tools["fixture-tool@"+s.toolVersion]), physical(t, found), "only this component's tool version is on its PATH")
		require.NotEmpty(t, probes)
		for _, probe := range probes {
			require.Equal(t, physical(t, pinned), physical(t, probe.Executable))
			require.Equal(t, s.identity, probe.Profile)
			require.Equal(t, s.stackValue, probe.Custom)
		}
	}
	for _, s := range []toolchainScenario{scenarios[0], scenarios[1], scenarios[0]} {
		check(t, s)
	}
	// The ambient process environment is never mutated to select an identity or tool.
	require.Equal(t, harness.ambient, os.Getenv("PATH"))
	require.Equal(t, "ambient", os.Getenv("CUSTOM_VALUE"))
	require.Equal(t, "ambient-token-must-not-return", os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE"))
}

// Authentication failure and cancellation stop the pipeline before the toolchain or the evaluator run.
func TestConfigurationToolchainStopsOnAuthenticationFailure(t *testing.T) {
	harness := newToolchainHarness(t, "1.0.0")
	info := harness.info(t, &toolchainScenario{identity: "does-not-exist", tofuVersion: "1.0.0", toolVersion: "1.0.0"}, filepath.Join(t.TempDir(), "probe.jsonl"))
	harness.events = nil
	_, err := harness.executor.prepareConfigurationEnvironment(t.Context(), harness.config, info, true)
	require.Error(t, err)
	require.Equal(t, []string{"authenticate"}, harness.events, "the toolchain and the evaluator must not run after an auth failure")
	require.Empty(t, harness.provider.requests)
	require.NotErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
}

// Every Terraform operation, including a bare init and the implicit init and workspace steps
// that precede plan, apply, and destroy, runs the executable pinned through dependencies.tools.
func TestConfigurationToolchainPinsExecutableForEveryOperation(t *testing.T) {
	harness := newToolchainHarness(t, "1.0.0", "2.0.0")
	for _, subCommand := range []string{"init", "plan", "apply", "destroy", "output", "validate"} {
		t.Run(subCommand, func(t *testing.T) {
			info, probes, request := harness.run(t, &toolchainScenario{identity: "development", tofuVersion: "2.0.0", toolVersion: "2.0.0", stackValue: "v", subCommand: subCommand})
			pinned := harness.tools["tofu@2.0.0"]
			require.Equal(t, pinned, info.Command)
			require.Equal(t, pinned, request.Invocation.Environment["ATMOS_TERRAFORM_BINARY"])
			require.Equal(t, subCommand, request.Operation.Command)
			require.NotEmpty(t, probes)
			sawOperation := false
			for _, probe := range probes {
				require.Equal(t, physical(t, pinned), physical(t, probe.Executable), "every subprocess uses the pinned executable: %v", probe.Arguments)
				require.Equal(t, request.Invocation.Environment["PATH"], probe.Path, "evaluation and execution share one PATH: %v", probe.Arguments)
				sawOperation = sawOperation || (len(probe.Arguments) > 0 && probe.Arguments[0] == subCommand)
			}
			require.True(t, sawOperation, "the %s operation itself must run", subCommand)
		})
	}
}

// A PATH set in the stack env section must survive behind the toolchain directories, so a
// component can still reach its own tools. The toolchain directories come first.
func TestConfigurationToolchainPreservesStackPathEntries(t *testing.T) {
	harness := newToolchainHarness(t, "1.0.0")
	custom := filepath.Join(t.TempDir(), "custom-bin")
	_, probes, request := harness.run(t, &toolchainScenario{identity: "development", tofuVersion: "1.0.0", toolVersion: "1.0.0", stackPath: custom})

	for name, path := range map[string]string{"plugin boundary": request.Invocation.Environment["PATH"], "terraform subprocess": probes[0].Path} {
		entries := filepath.SplitList(path)
		require.Contains(t, entries, custom, "%s: the stack PATH entry must not be discarded", name)
		_, ok := pathContains(path)
		require.True(t, ok, "%s: the toolchain tool must still be reachable", name)
		require.Less(t, indexOf(entries, filepath.Dir(harness.tools["tofu@1.0.0"])), indexOf(entries, custom), "%s: toolchain precedes stack PATH", name)
	}
}

func indexOf(values []string, want string) int {
	for i, value := range values {
		if value == want {
			return i
		}
	}
	return -1
}

// toolchainPathEnv decides which PATH wins; exercise every source against a real toolchain environment.
func TestToolchainPathEnvSelectsThePathToExtend(t *testing.T) {
	harness := newToolchainHarness(t, "1.0.0")
	info := harness.info(t, &toolchainScenario{identity: "development", tofuVersion: "1.0.0", toolVersion: "1.0.0"}, filepath.Join(t.TempDir(), "probe.jsonl"))
	tenv, err := resolveAndInstallToolchainDeps(harness.config, info)
	require.NoError(t, err)
	dirs := strings.Join(tenv.ToolchainDirs(), string(os.PathListSeparator))
	require.NotEmpty(t, dirs)
	ambient := os.Getenv("PATH")
	join := func(parts ...string) string { return strings.Join(parts, string(os.PathListSeparator)) }

	for _, test := range []struct {
		name      string
		component []string
		sanitized []string
		want      string
	}{
		{name: "no selected PATH keeps the toolchain PATH", want: join(dirs, ambient)},
		{name: "selected equal to the ambient PATH", component: []string{"PATH=" + ambient}, want: join(dirs, ambient)},
		{name: "component PATH is extended", component: []string{"PATH=/component"}, want: join(dirs, "/component")},
		{name: "last component PATH wins", component: []string{"PATH=/first", "PATH=/last"}, want: join(dirs, "/last")},
		{name: "identity PATH is extended", sanitized: []string{"AWS_PROFILE=x", "PATH=/identity"}, want: join(dirs, "/identity")},
		{name: "component PATH beats identity PATH", component: []string{"PATH=/component"}, sanitized: []string{"PATH=/identity"}, want: join(dirs, "/component")},
		{name: "explicitly empty PATH leaves only toolchain directories", component: []string{"PATH="}, want: dirs},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := toolchainPathEnv(&schema.ConfigAndStacksInfo{ComponentEnvList: test.component, SanitizedEnv: test.sanitized}, tenv)
			require.Equal(t, []string{"PATH=" + test.want}, got)
		})
	}

	// Windows names the variable Path or PATH interchangeably; other platforms keep them distinct.
	for _, test := range []struct {
		name      string
		goos      string
		component []string
		sanitized []string
		want      string
	}{
		{name: "windows component Path is extended", goos: "windows", component: []string{"Path=/component"}, want: join(dirs, "/component")},
		{name: "windows identity Path is extended", goos: "windows", sanitized: []string{"Path=/identity"}, want: join(dirs, "/identity")},
		{name: "windows last case variant wins", goos: "windows", component: []string{"PATH=/first", "Path=/last"}, want: join(dirs, "/last")},
		{name: "windows component Path beats identity PATH", goos: "windows", component: []string{"Path=/component"}, sanitized: []string{"PATH=/identity"}, want: join(dirs, "/component")},
		{name: "linux Path is not PATH", goos: "linux", component: []string{"Path=/component"}, want: join(dirs, ambient)},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := toolchainPathEnvForGOOS(&schema.ConfigAndStacksInfo{ComponentEnvList: test.component, SanitizedEnv: test.sanitized}, tenv, test.goos)
			require.Equal(t, []string{"PATH=" + test.want}, got)
		})
	}

	t.Run("without toolchain directories nothing is overridden", func(t *testing.T) {
		empty := &dependencies.ToolchainEnvironment{}
		require.Nil(t, toolchainPathEnv(&schema.ConfigAndStacksInfo{ComponentEnvList: []string{"PATH=/component"}}, empty))
	})
}

// An identity may carry its own PATH; the toolchain directories extend it for both the
// evaluator and Terraform instead of replacing it with the host PATH.
func TestConfigurationToolchainPreservesIdentityPath(t *testing.T) {
	harness := newToolchainHarness(t, "1.0.0")
	identityBin := filepath.Join(t.TempDir(), "identity-bin")
	harness.afterAuth = func(info *schema.ConfigAndStacksInfo) {
		info.SanitizedEnv = append(info.SanitizedEnv, "PATH="+identityBin+string(os.PathListSeparator)+harness.ambient)
	}
	_, probes, request := harness.run(t, &toolchainScenario{identity: "development", tofuVersion: "1.0.0", toolVersion: "1.0.0"})
	for name, path := range map[string]string{"plugin boundary": request.Invocation.Environment["PATH"], "terraform subprocess": probes[0].Path} {
		entries := filepath.SplitList(path)
		require.Contains(t, entries, identityBin, name)
		_, ok := pathContains(path)
		require.True(t, ok, name)
		require.Less(t, indexOf(entries, filepath.Dir(harness.tools["tofu@1.0.0"])), indexOf(entries, identityBin), name)
	}
}

// unitHarness prepares a toolchain harness whose pinned terragrunt is the test binary, plus the unit
// directory executable mode runs in.
func unitHarness(t *testing.T) (*toolchainHarness, *schema.ConfigAndStacksInfo, string) {
	t.Helper()
	harness := newToolchainHarness(t, "1.0.0")
	harness.config.Toolchain.Aliases["terragrunt"] = "gruntwork-io/terragrunt"
	pinned := installToolchainFixture(t, harness.config.Toolchain.InstallPath, "gruntwork-io", "terragrunt", "1.2.1", "terragrunt")
	unit := filepath.Join(harness.config.BasePathAbsolute, "live", "dev", "app")
	require.NoError(t, os.MkdirAll(unit, 0o755))
	probe := filepath.Join(t.TempDir(), "probe.jsonl")
	info := harness.info(t, &toolchainScenario{identity: "development", tofuVersion: "1.0.0", toolVersion: "1.0.0"}, probe)
	info.ComponentSettingsSection = map[string]any{"configuration": map[string]any{"engine": "terragrunt", "provider": "terragrunt", "source": "live/dev/app/terragrunt.hcl", schema.ConfigurationOwnedKey: true}}
	info.AdditionalArgsAndFlags = []string{"-no-color"}
	tools, _ := info.ComponentSection["dependencies"].(map[string]any)["tools"].(map[string]any)
	tools["terragrunt"] = "1.2.1"
	return harness, info, pinned
}

// unitRun is how ExecuteTerraform reaches executable mode for a component without test fixtures.
var unitRun = terragruntUnitRun{shouldProcess: true, runHooks: true}

// Executable mode runs the pinned terragrunt in the unit directory, with the identity and toolchain Atmos
// resolved, and never touches native preparation.
func TestExecuteTerragruntUnitRunsThePinnedExecutableInTheUnit(t *testing.T) {
	harness, info, pinned := unitHarness(t)
	probePath := info.ComponentEnvSection[testEnvToolchainProbe].(string)
	info.ComponentProviderOptions = map[string]map[string]schema.ComponentProviderOption{"terragrunt": {
		"feature":                            {Value: map[string]any{"alpha": "true"}},
		"iam_assume_role_web_identity_token": {Value: "token-value", Sensitive: true},
	}}
	harness.config.Components.Terraform.ConfigurationProviders = map[string]schema.ComponentConfiguration{
		"terragrunt": {Enabled: true, Mode: "executable", Options: map[string]any{"features": map[string]any{"beta": "from-config"}}},
	}

	require.NoError(t, harness.executor.executeTerragruntUnit(t.Context(), harness.config, info, unitRun))

	probes := readToolchainProbes(t, probePath)
	require.Len(t, probes, 2, "one version probe and one unit command")
	require.Equal(t, []string{"--version"}, probes[0].Arguments)
	run := probes[1]
	unit := filepath.Join(harness.config.BasePathAbsolute, "live", "dev", "app")
	require.Equal(t, physical(t, pinned), physical(t, run.Executable), "the toolchain-pinned terragrunt runs, not the ambient PATH")
	require.Equal(t, physical(t, unit), physical(t, run.Directory))
	require.Equal(t, []string{"--working-dir", unit, "run", "--feature", "alpha=true", "--feature", "beta=from-config", "--", "plan", "-no-color"}, run.Arguments)
	require.Equal(t, "true", run.NonInteractive)
	require.Equal(t, harness.tools["tofu@1.0.0"], run.TFPath, "terragrunt runs the engine Atmos selected")
	require.Equal(t, "development", run.Profile, "the Atmos identity reaches the unit")
	require.Equal(t, "token-value", run.WebIdentity)
	for _, argument := range run.Arguments {
		require.NotContains(t, argument, "token-value", "a sensitive option never appears in argv")
	}
	require.Equal(t, []string{"authenticate", "toolchain"}, harness.events, "native preparation never starts")
}

func TestExecuteTerragruntUnitPropagatesTheUnitExitCode(t *testing.T) {
	harness, info, _ := unitHarness(t)
	info.ComponentEnvSection[testEnvProbeExit] = "3"
	err := harness.executor.executeTerragruntUnit(t.Context(), harness.config, info, unitRun)
	var exit errUtils.ExitCodeError
	require.ErrorAs(t, err, &exit)
	require.Equal(t, 3, exit.Code)
}

func TestExecuteTerragruntUnitRejectsWhatItCannotHonorBeforeRunning(t *testing.T) {
	harness, info, _ := unitHarness(t)
	probePath := info.ComponentEnvSection[testEnvToolchainProbe].(string)
	info.ComponentVarsSection = map[string]any{"region": "us-east-1"}
	require.ErrorIs(t, harness.executor.executeTerragruntUnit(t.Context(), harness.config, info, unitRun), errUtils.ErrExecutableModeUnsupported)
	require.Empty(t, readToolchainProbes(t, probePath), "nothing ran")
}

func TestExecuteTerragruntUnitExplainsAMissingExecutable(t *testing.T) {
	harness, info, _ := unitHarness(t)
	delete(info.ComponentSection["dependencies"].(map[string]any)["tools"].(map[string]any), "terragrunt")
	err := harness.executor.executeTerragruntUnit(t.Context(), harness.config, info, unitRun)
	require.ErrorIs(t, err, errUtils.ErrExecutableNotFound)
}

func TestExecuteTerragruntUnitDryRunRunsNothing(t *testing.T) {
	harness, info, _ := unitHarness(t)
	probePath := info.ComponentEnvSection[testEnvToolchainProbe].(string)
	info.DryRun = true
	require.NoError(t, harness.executor.executeTerragruntUnit(t.Context(), harness.config, info, unitRun))
	require.Empty(t, readToolchainProbes(t, probePath))
}

func TestProviderSectionFollowsTheProviderName(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.ConfigurationProviders = map[string]schema.ComponentConfiguration{
		"tg":    {Enabled: true, Provider: "terragrunt"},
		"other": {Enabled: false, Provider: "terragrunt"},
	}
	require.Equal(t, "tg", providerSection(config, "terragrunt"))
	require.Equal(t, "unknown", providerSection(config, "unknown"))
}

// Compile-time sentinel for the record field these tests depend on.
var _ = configurationPlan{Modules: true}

var (
	errModuleDownload    = errors.New("module source unreachable")
	errUnexpectedCommand = errors.New("unexpected command")
)

// reviewWithModules is reviewInDefaultWorkdir for a unit whose `init` downloaded a child module.
func (p *reviewedPlanProject) reviewWithModules(t *testing.T, module string) string {
	t.Helper()
	p.explicitWorkdir = false
	info := p.info(t, "plan")
	p.provider.EXPECT().Prepare(gomock.Any(), info).Return(p.prepared(), nil)
	execution, err := testExecutor(t).prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, p.config, info, false)
	require.NoError(t, err)
	p.workdir = execution.workingDir
	writeModule(t, p.workdir, module)
	planPath := filepath.Join(p.workdir, "reviewed.planfile")
	require.NoError(t, os.WriteFile(planPath, []byte("reviewed plan"), 0o600))
	observeFixturePlan(t, p.config, info, execution, planPath)
	require.NoError(t, recordConfigurationPlan(t.Context(), p.config, info, execution, []string{"-out=" + planPath}))
	return planPath
}

func writeModule(t *testing.T, workdir, content string) {
	t.Helper()
	module := filepath.Join(workdir, downloadedModulesDir, "vpc", "main.tf")
	require.NoError(t, os.MkdirAll(filepath.Dir(module), 0o755))
	require.NoError(t, os.WriteFile(module, []byte(content), 0o600))
}

func readPlanRecord(t *testing.T, planPath string) configurationPlan {
	t.Helper()
	data, err := os.ReadFile(planPath + configurationPlanSuffix)
	require.NoError(t, err)
	var record configurationPlan
	require.NoError(t, json.Unmarshal(data, &record))
	return record
}

// moduleGetter uses the generated shell mock for `terraform get`: it installs the given module content into
// the directory it runs in, or fails.
func moduleGetter(t *testing.T, content string, failure error) *shellFixture {
	t.Helper()
	return newShellFixture(t, func(call *shellCall) error {
		if !slices.Equal(call.args, []string{"get"}) {
			return errUnexpectedCommand
		}
		if failure != nil {
			return failure
		}
		module := filepath.Join(call.dir, downloadedModulesDir, "vpc", "main.tf")
		if err := os.MkdirAll(filepath.Dir(module), 0o755); err != nil {
			return err
		}
		return os.WriteFile(module, []byte(content), 0o600)
	})
}

// callDirs lists the directory of every call.
func callDirs(shell *shellFixture) []string {
	dirs := make([]string, 0, len(shell.calls))
	for i := range shell.calls {
		dirs = append(dirs, shell.calls[i].dir)
	}
	return dirs
}

// A pinned module downloads the same bytes on a fresh runner, so the rebuilt workdir verifies.
func TestApplyOfRestoredPlanDownloadsPinnedModulesBeforeVerification(t *testing.T) {
	project := newReviewedPlanProject(t)
	planPath := project.reviewWithModules(t, "# vpc v1.2.3")
	require.True(t, readPlanRecord(t, planPath).Modules, "the record notes the downloaded modules")
	project.freshRunner(t, planPath)
	getter := moduleGetter(t, "# vpc v1.2.3", nil)
	testExecutor(t).services.ExecuteShellCommand = getter.mock.Execute

	_, execution, err := project.freshApply(t, planPath, project.prepared())

	require.NoError(t, err)
	require.Equal(t, []string{project.workdir}, callDirs(getter), "modules download once, into the rebuilt workdir")
	require.Equal(t, project.workdir, execution.componentPath)
}

// A floating source that now resolves to different code rejects the plan: the modules stay in the digest.
func TestApplyOfRestoredPlanRejectsChangedModules(t *testing.T) {
	project := newReviewedPlanProject(t)
	planPath := project.reviewWithModules(t, "# vpc reviewed")
	project.freshRunner(t, planPath)
	testExecutor(t).services.ExecuteShellCommand = moduleGetter(t, "# vpc moved branch", nil).mock.Execute

	_, _, err := project.freshApply(t, planPath, project.prepared())

	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
}

// A download failure rejects the plan rather than skipping the module check.
func TestApplyOfRestoredPlanRejectsUnreachableModules(t *testing.T) {
	project := newReviewedPlanProject(t)
	planPath := project.reviewWithModules(t, "# vpc reviewed")
	project.freshRunner(t, planPath)
	testExecutor(t).services.ExecuteShellCommand = moduleGetter(t, "", errModuleDownload).mock.Execute

	_, _, err := project.freshApply(t, planPath, project.prepared())

	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	require.ErrorIs(t, err, errModuleDownload)
}

// A plan made without downloaded modules never downloads anything on a fresh runner.
func TestApplyOfRestoredPlanWithoutModulesDoesNotDownload(t *testing.T) {
	project := newReviewedPlanProject(t)
	planPath := project.reviewInDefaultWorkdir(t)
	require.False(t, readPlanRecord(t, planPath).Modules)
	project.freshRunner(t, planPath)
	getter := moduleGetter(t, "", nil)
	testExecutor(t).services.ExecuteShellCommand = getter.mock.Execute

	_, _, err := project.freshApply(t, planPath, project.prepared())

	require.NoError(t, err)
	require.Empty(t, getter.calls)
}

// The module download runs with the sanitized environment, and keeps the default one when none is set.
func TestInstallReviewedModulesUsesSanitizedEnvironment(t *testing.T) {
	rebuild := &reviewedRebuild{plan: &configurationPlan{Modules: true}}
	for name, env := range map[string][]string{"sanitized": {"PATH=/sanitized", "TF_VAR_x=1"}, "unset": nil} {
		t.Run(name, func(t *testing.T) {
			getter := moduleGetter(t, "# vpc", nil)
			executor := testExecutor(t)
			executor.services.ExecuteShellCommand = getter.mock.Execute
			info := &schema.ConfigAndStacksInfo{Command: "terraform", SanitizedEnv: env}

			require.NoError(t, executor.installReviewedModules(&schema.AtmosConfiguration{}, info, rebuild, t.TempDir()))

			require.Len(t, getter.calls, 1)
			require.Equal(t, env, getter.calls[0].options.ProcessEnv)
		})
	}
}

func TestRestoredPlanRejectsModulesChangedByInit(t *testing.T) {
	project := newReviewedPlanProject(t)
	planPath := project.reviewWithModules(t, "# reviewed module")
	project.freshRunner(t, planPath)
	executor := testExecutor(t)
	executor.services.ExecuteShellCommand = moduleGetter(t, "# reviewed module", nil).mock.Execute
	info, execution, err := project.freshApply(t, planPath, project.prepared())
	require.NoError(t, err)
	project.config.Components.Terraform.Init.Upgrade = schema.TerraformInitUpgradeAlways
	info.ComponentBackendType = "http"
	var initialized, applied bool
	shell := newShellFixture(t, func(call *shellCall) error {
		if call.args[0] == "init" {
			require.Contains(t, call.args, "-upgrade")
			initialized = true
			writeModule(t, call.dir, "# upgraded module")
		}
		if call.args[0] == "apply" {
			applied = true
		}
		return nil
	})
	executor.services.ExecuteShellCommand = shell.mock.Execute
	err = executor.executeCommandPipeline(project.config, info, execution)
	require.True(t, initialized)
	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	require.False(t, applied, "apply must not run against modules changed by init")
}

func TestRestoredPlanRejectsModulesChangedByInitRecovery(t *testing.T) {
	for _, hooks := range []bool{false, true} {
		t.Run(fmt.Sprint(hooks), func(t *testing.T) {
			project := newReviewedPlanProject(t)
			planPath := project.reviewWithModules(t, "# reviewed module")
			project.freshRunner(t, planPath)
			executor := testExecutor(t)
			executor.services.ExecuteShellCommand = moduleGetter(t, "# reviewed module", nil).mock.Execute
			info, execution, err := project.freshApply(t, planPath, project.prepared())
			require.NoError(t, err)
			if hooks {
				info.ComponentExecution = &schema.ComponentExecution{BeforeHooks: []schema.ComponentHook{execHook("before", "apply")}}
			}
			attempts, initialized := 0, false
			executor.services.ExecuteShellCommand = newShellFixture(t, func(call *shellCall) error {
				if len(call.args) > 0 && call.args[0] == "apply" {
					attempts++
					_, _ = io.WriteString(call.options.ExecMetadataStderrCapture, "Error: Inconsistent dependency lock file")
					return errUtils.ExitCodeError{Code: 1}
				}
				return nil
			}).mock.Execute
			project.config.Components.Terraform.Init.Upgrade = schema.TerraformInitUpgradeAlways
			executor.executeTerraformInitForcedFn = func(_ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo, dir, _ string, _, upgrade bool, _ ...ShellCommandOption) error {
				require.True(t, upgrade)
				initialized = true
				writeModule(t, dir, "# upgraded module")
				return nil
			}
			err = executor.runMainCommandPhase(project.config, info, execution, []string{"apply"}, execution.componentPath, false)
			require.True(t, initialized)
			require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
			require.Equal(t, 1, attempts, "recovery must not retry against changed modules")
		})
	}
}

func TestRestoredPlanRetriesAfterModuleDownloadFailure(t *testing.T) {
	project := newReviewedPlanProject(t)
	planPath := project.reviewWithModules(t, "# reviewed module")
	project.freshRunner(t, planPath)
	plan, err := os.ReadFile(planPath)
	require.NoError(t, err)
	record, err := os.ReadFile(planPath + configurationPlanSuffix)
	require.NoError(t, err)
	executor := testExecutor(t)
	executor.services.ExecuteShellCommand = newShellFixture(t, func(call *shellCall) error {
		require.Equal(t, []string{"get"}, call.args)
		writeModule(t, call.dir, "# incomplete download")
		return errModuleDownload
	}).mock.Execute
	_, _, err = project.freshApply(t, planPath, project.prepared())
	require.ErrorIs(t, err, errModuleDownload)
	failed, err := filepath.Glob(filepath.Join(filepath.Dir(project.workdir), ".atmos-failed-rebuild-*", "workdir", downloadedModulesDir, "vpc", "main.tf"))
	require.NoError(t, err)
	require.Len(t, failed, 1, "unverified output is preserved separately from restored artifacts")
	partial, err := os.ReadFile(failed[0])
	require.NoError(t, err)
	require.Equal(t, "# incomplete download", string(partial))
	restored, err := os.ReadFile(planPath)
	require.NoError(t, err)
	require.Equal(t, plan, restored)
	restored, err = os.ReadFile(planPath + configurationPlanSuffix)
	require.NoError(t, err)
	require.Equal(t, record, restored)
	getter := moduleGetter(t, "# reviewed module", nil)
	executor.services.ExecuteShellCommand = getter.mock.Execute
	_, execution, err := project.freshApply(t, planPath, project.prepared())
	require.NoError(t, err)
	require.Equal(t, project.workdir, execution.workingDir)
	require.Len(t, getter.calls, 1, "retry reaches the module download")
}
