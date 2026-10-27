package create

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component/custom"
	"github.com/cloudposse/atmos/pkg/generator/engine"
	"github.com/cloudposse/atmos/pkg/generator/setup"
	generatorui "github.com/cloudposse/atmos/pkg/generator/ui"
	runnerstep "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
)

func fixture(t *testing.T) (*schema.AtmosConfiguration, Renderer) {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)
	cfg := &schema.AtmosConfiguration{BasePathAbsolute: root, StacksBaseAbsolutePath: filepath.Join(root, "stacks"), Stacks: schema.Stacks{BasePath: "stacks", IncludedPaths: []string{"deploy/**/*"}, ExcludedPaths: []string{"**/_*"}}}
	gen, err := setup.NewGeneratorContext()
	require.NoError(t, err)
	return cfg, gen.UI
}

func write(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestMinimalCreationAndIndependentHistory(t *testing.T) {
	cfg, renderer := fixture(t)
	// Existing project answers and files must not be imported or replaced.
	write(t, filepath.Join(cfg.BasePathAbsolute, ".atmos", "scaffold.yaml"), "unrelated project record")
	write(t, filepath.Join(cfg.BasePathAbsolute, "stacks", "catalog", "defaults.yaml"), "vars:\n  region: us-east-1\n")
	opts := &Options{File: "stacks/deploy/dev.yaml", Name: "dev", Imports: []string{"catalog/defaults"}, Components: []string{"terraform:network", "helmfile:services"}, Defaults: true}
	plan, err := Prepare(cfg, opts, renderer)
	require.NoError(t, err)
	require.Len(t, plan.Generation.Files, 2)
	assert.NoFileExists(t, filepath.Join(cfg.BasePathAbsolute, "stacks", "deploy", "dev.yaml"))
	manifest := plan.Generation.Files[0].Content
	assert.Contains(t, manifest, "enabled: false")
	assert.Contains(t, manifest, "network")
	assert.Contains(t, manifest, "services")
	assert.Contains(t, manifest, "catalog/defaults")
	require.NoError(t, plan.Apply(context.Background(), nil))
	written, err := os.ReadFile(opts.File)
	require.NoError(t, err)
	assert.Equal(t, manifest, string(written))
	history, err := os.ReadFile(filepath.Join(plan.Root, plan.Generation.Files[1].Path))
	require.NoError(t, err)
	assert.Contains(t, string(history), "manifest: stacks/deploy/dev.yaml")
	opts.File, opts.Name = "stacks/deploy/prod.yaml", "prod"
	second, err := Prepare(cfg, opts, renderer)
	require.NoError(t, err)
	assert.NotEqual(t, plan.Generation.Files[1].Path, second.Generation.Files[1].Path)
	require.NoError(t, second.Apply(context.Background(), nil))
	preserved, err := os.ReadFile(filepath.Join(plan.Root, plan.Generation.Files[1].Path))
	require.NoError(t, err)
	assert.Equal(t, history, preserved)
	original, err := os.ReadFile(filepath.Join(plan.Root, ".atmos", "scaffold.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "unrelated project record", string(original))
	_, err = Prepare(cfg, opts, renderer)
	require.Error(t, err)
}

func TestCreationPreservesNamingConvention(t *testing.T) {
	cfg, renderer := fixture(t)
	plan, err := Prepare(cfg, &Options{File: "stacks/deploy/dev.yaml", Defaults: true}, renderer)
	require.NoError(t, err)
	var manifest map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(plan.Generation.Files[0].Content), &manifest))
	assert.NotContains(t, manifest, "name")
}

const customDefinition = `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: application-stack
spec:
  stack:
    manifest: '{{ .Config.stack_file }}'
  fields:
    - name: environment
      type: input
      required: true
    - name: sources
      type: confirm
      default: false
  files:
    - path: stack.yaml
      target: '{{ .Config.stack_file }}'
    - path: components/terraform/network/main.tf
      when: answers.sources == true
  hooks:
    notice:
      events: [after.scaffold.generate]
      kind: step
      type: create-test-unregistered-hook
      with:
        message: 'Would run for {{ .Answers.environment }}'
`

func customFixture(t *testing.T, definition string) {
	t.Helper()
	write(t, "atmos.yaml", "scaffold:\n  templates:\n    application-stack:\n      source: ./scaffolds/app\n")
	write(t, "scaffolds/app/scaffold.yaml", definition)
	write(t, "scaffolds/app/stack.yaml", `# atmos:template
vars:
  stage: {{ .Config.environment | quote }}
components:
  terraform:
    network:
      vars: {}
`)
	write(t, "scaffolds/app/components/terraform/network/main.tf", "# source stub\n")
}

func TestConfiguredScaffoldUsesSharedRenderer(t *testing.T) {
	cfg, renderer := fixture(t)
	customFixture(t, customDefinition)
	opts := &Options{File: "stacks/deploy/dev.yaml", Template: "application-stack", Defaults: true, Values: map[string]any{"environment": "dev", "stack_region": "us-east-1"}}
	plan, err := Prepare(cfg, opts, renderer)
	require.NoError(t, err)
	require.Len(t, plan.Generation.Files, 2) // Source file disabled by the scaffold condition.
	assert.Contains(t, plan.Generation.Files[0].Content, `stage: "dev"`)
	assert.NotContains(t, plan.Generation.Files[0].Content, "enabled: false")
	assert.Contains(t, plan.Hooks, "notice")
	assert.Equal(t, "us-east-1", plan.Generation.Values["stack_region"])
	assert.NoDirExists(t, filepath.Join(cfg.BasePathAbsolute, "stacks"))
	assert.NoDirExists(t, filepath.Join(cfg.BasePathAbsolute, "components"))
	// The invalid hook type proves planning never executes hooks; skipping uses
	// the normal scaffold predicate during real creation.
	require.NoError(t, plan.Apply(context.Background(), func(string) bool { return true }))
	opts.File = "stacks/deploy/staging.yaml"
	opts.Values["sources"] = true
	next, err := Prepare(cfg, opts, renderer)
	require.NoError(t, err)
	require.Len(t, next.Generation.Files, 3)
	require.NoError(t, next.Apply(context.Background(), func(string) bool { return true }))
	content, err := os.ReadFile("components/terraform/network/main.tf")
	require.NoError(t, err)
	assert.Equal(t, "# source stub\n", string(content))
	opts.File = "stacks/deploy/prod.yaml"
	_, err = Prepare(cfg, opts, renderer)
	require.Error(t, err) // Shared component source is never overwritten.
	assert.NoFileExists(t, opts.File)
}

func TestPreflightFailures(t *testing.T) {
	for _, tc := range []struct {
		name             string
		options          Options
		definition, body string
		want             string
	}{
		{name: "missing file", want: "--file or --target is required"},
		{name: "extension", options: Options{File: "stacks/deploy/dev.txt"}, want: ".yaml"},
		{name: "excluded", options: Options{File: "stacks/deploy/_defaults.yaml"}, want: "excluded_paths"},
		{name: "undiscovered", options: Options{File: "stacks/catalog/dev.yaml"}, want: "included_paths"},
		{name: "outside stacks", options: Options{File: "elsewhere/dev.yaml"}, want: "stacks.base_path"},
		{name: "unknown kind", options: Options{File: "stacks/deploy/dev.yaml", Components: []string{"typo:network"}}, want: "supported kind:name"},
		{name: "duplicate", options: Options{File: "stacks/deploy/dev.yaml", Components: []string{"terraform:x", "terraform:x"}}, want: "duplicate"},
		{name: "reserved", options: Options{File: "stacks/deploy/dev.yaml", Values: map[string]any{"stack_name": "hidden"}}, want: "reserved"},
		{name: "missing answer", options: Options{File: "stacks/deploy/dev.yaml", Template: "application-stack"}, definition: customDefinition, want: "environment"},
		{name: "incompatible", options: Options{File: "stacks/deploy/dev.yaml", Template: "application-stack"}, definition: strings.Replace(customDefinition, "  stack:\n    manifest: '{{ .Config.stack_file }}'\n", "", 1), want: "spec.stack.manifest"},
		{name: "wrong output", options: Options{File: "stacks/deploy/dev.yaml", Template: "application-stack", Values: map[string]any{"environment": "dev"}}, definition: strings.Replace(customDefinition, "manifest: '{{ .Config.stack_file }}'", "manifest: other.yaml", 1), want: "must match --file"},
		{name: "invalid manifest", options: Options{File: "stacks/deploy/dev.yaml", Template: "application-stack", Values: map[string]any{"environment": "dev"}}, definition: customDefinition, body: "vars: 5\n", want: "generated stack"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, renderer := fixture(t)
			if tc.definition != "" {
				customFixture(t, tc.definition)
			}
			if tc.body != "" {
				write(t, "scaffolds/app/stack.yaml", tc.body)
			}
			tc.options.Defaults = true
			_, err := Prepare(cfg, &tc.options, renderer)
			require.ErrorContains(t, err, tc.want)
			assert.NoDirExists(t, filepath.Join(cfg.BasePathAbsolute, "stacks"))
			assert.NoDirExists(t, filepath.Join(cfg.BasePathAbsolute, ".atmos"))
		})
	}
}

func TestCancellationAndRacePreserveFiles(t *testing.T) {
	cfg, renderer := fixture(t)
	plan, err := Prepare(cfg, &Options{File: "stacks/deploy/dev.yaml", Defaults: true}, renderer)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, plan.Apply(ctx, nil), context.Canceled)
	assert.NoDirExists(t, filepath.Join(cfg.BasePathAbsolute, "stacks"))
	write(t, "stacks/deploy/dev.yaml", "created by someone else")
	require.Error(t, plan.Apply(context.Background(), nil))
	actual, err := os.ReadFile("stacks/deploy/dev.yaml")
	require.NoError(t, err)
	assert.Equal(t, "created by someone else", string(actual))
	assert.NoDirExists(t, filepath.Join(cfg.BasePathAbsolute, ".atmos"))
}

// observationHook verifies hook timing without invoking external executables.
type observationHook struct {
	runnerstep.BaseHandler
	events   *[]string
	manifest string
}

func (h *observationHook) Validate(*schema.WorkflowStep) error { return nil }

func (h *observationHook) Execute(_ context.Context, step *schema.WorkflowStep, vars *runnerstep.Variables) (*runnerstep.StepResult, error) {
	content, err := vars.Resolve(step.Content)
	if err != nil {
		return nil, err
	}
	_, statErr := os.Stat(h.manifest)
	if statErr == nil {
		content += " exists"
	} else {
		content += " absent"
	}
	*h.events = append(*h.events, content)
	return runnerstep.NewStepResult(content), nil
}

func TestScaffoldHooksWrapPlannedWrites(t *testing.T) {
	cfg, renderer := fixture(t)
	events := []string{}
	runnerstep.Register(&observationHook{BaseHandler: runnerstep.NewBaseHandler("stack-create-observation", runnerstep.CategoryOutput, false), events: &events, manifest: "stacks/deploy/dev.yaml"})
	definition := strings.Split(customDefinition, "  hooks:\n")[0] + `  hooks:
    before:
      events: [before.scaffold.generate]
      kind: step
      type: stack-create-observation
      with:
        content: 'before {{ .Answers.environment }}'
    after:
      events: [after.scaffold.generate]
      kind: step
      type: stack-create-observation
      with:
        content: 'after {{ .Answers.environment }}'
`
	customFixture(t, definition)
	plan, err := Prepare(cfg, &Options{File: "stacks/deploy/dev.yaml", Template: "application-stack", Values: map[string]any{"environment": "dev"}, Defaults: true}, renderer)
	require.NoError(t, err)
	assert.Empty(t, events)
	require.NoError(t, plan.Apply(context.Background(), nil))
	assert.Equal(t, []string{"before dev absent", "after dev exists"}, events)
}

func TestExtensionlessDiscoveryPattern(t *testing.T) {
	cfg, renderer := fixture(t)
	cfg.Stacks.IncludedPaths = []string{"deploy/dev"}
	_, err := Prepare(cfg, &Options{File: "stacks/deploy/dev.yaml", Defaults: true}, renderer)
	require.NoError(t, err)
}

func TestCreationResolvesFromProjectRootOutsideCWD(t *testing.T) {
	for name, configPath := range map[string]func(root string) string{
		"loaded config directory":       func(root string) string { return root },
		"multiple config files":         func(root string) string { return root + ";" + t.TempDir() },
		"no recorded config path":       func(string) string { return "" },
		"config directory lacks a file": func(string) string { return t.TempDir() },
	} {
		t.Run(name, func(t *testing.T) {
			cfg, renderer := fixture(t)
			customFixture(t, customDefinition)
			cfg.CliConfigPath = configPath(cfg.BasePathAbsolute)
			// Run from an unrelated directory: --file and the configured template
			// source must resolve from the project, not from the CWD.
			t.Chdir(t.TempDir())
			opts := &Options{File: filepath.Join("stacks", "deploy", "dev.yaml"), Template: "application-stack", Defaults: true, Values: map[string]any{"environment": "dev"}}
			plan, err := Prepare(cfg, opts, renderer)
			require.NoError(t, err)
			assert.Equal(t, filepath.Join("stacks", "deploy", "dev.yaml"), plan.Manifest)
			assert.Contains(t, plan.Generation.Files[0].Content, `stage: "dev"`)
		})
	}
}

func TestAbsoluteFileOutsideProjectIsRejected(t *testing.T) {
	cfg, renderer := fixture(t)
	outside := filepath.Join(t.TempDir(), "stacks", "deploy", "dev.yaml")
	_, err := Prepare(cfg, &Options{File: outside, Defaults: true}, renderer)
	require.ErrorIs(t, err, errUtils.ErrInvalidFlag)
}

func TestCreationUsesProjectEditionForHooks(t *testing.T) {
	for _, pin := range []string{"", "2026-10-18"} {
		t.Run(pin, func(t *testing.T) {
			cfg, renderer := fixture(t)
			cfg.Edition = pin
			write(t, "recipe/stack.yaml.tmpl", "name: dev\n")
			write(t, "atmos.yaml", `scaffold:
  templates:
    custom:
      source: ./recipe
`)
			// The template output uses stack_file, just like the built-in template.
			write(t, "recipe/scaffold.yaml", `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: test
spec:
  stack:
    manifest: '{{ .Config.stack_file }}'
  files:
    - path: stack.yaml.tmpl
      target: '{{ .Config.stack_file }}'
  hooks:
    typo:
      events: [after.scaffold.genrate]
      kind: step
      type: shell
`)
			plan, err := Prepare(cfg, &Options{File: "stacks/deploy/dev.yaml", Template: "custom", Defaults: true}, renderer)
			if pin == "" {
				require.ErrorIs(t, err, errUtils.ErrUnsupportedHookEvent)
			} else {
				require.NoError(t, err)
				require.NoError(t, plan.Apply(context.Background(), nil))
				require.FileExists(t, "stacks/deploy/dev.yaml")
			}
		})
	}
}

func TestScaffoldConfigDirFallbackAndErrors(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(t.TempDir(), "missing")

	dir, err := scaffoldConfigDir(&schema.AtmosConfiguration{CliConfigPath: missing}, root)
	require.NoError(t, err)
	assert.Equal(t, root, dir, "a missing config directory falls back to the project root")

	// Errors other than a missing path must not silently select another catalog.
	_, err = scaffoldConfigDir(&schema.AtmosConfiguration{CliConfigPath: "bad\x00path"}, root)
	require.ErrorIs(t, err, errUtils.ErrInvalidConfig)
}

func TestTemplateSelectionFailuresStopBeforeRendering(t *testing.T) {
	for _, tc := range []struct {
		name       string
		template   string
		definition string
		wantIs     error
		want       string
	}{
		{name: "unknown template", template: "does-not-exist", wantIs: errUtils.ErrScaffoldNotFound, want: "does-not-exist"},
		{name: "malformed scaffold definition", template: "application-stack", definition: "apiVersion: atmos/v1\nkind: [unclosed\n", wantIs: errUtils.ErrScaffoldLoadConfig},
		{name: "reserved field name", template: "application-stack", definition: strings.Replace(customDefinition, "  fields:\n", "  fields:\n    - name: stack_name\n      type: input\n", 1), wantIs: errUtils.ErrInvalidConfig, want: "are reserved inputs"},
		{name: "unrenderable manifest path", template: "application-stack", definition: strings.Replace(customDefinition, "manifest: '{{ .Config.stack_file }}'", "manifest: '{{ .Config.missing.nested }}'", 1), wantIs: errUtils.ErrInvalidConfig, want: "must match --file"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, renderer := fixture(t)
			if tc.definition != "" {
				customFixture(t, tc.definition)
			}
			opts := &Options{File: "stacks/deploy/dev.yaml", Template: tc.template, Defaults: true, Values: map[string]any{"environment": "dev"}}
			_, err := Prepare(cfg, opts, renderer)
			require.ErrorIs(t, err, tc.wantIs)
			if tc.want != "" {
				assert.Contains(t, err.Error(), tc.want)
			}
			assertNothingWritten(t, cfg, opts.File)
			assert.NoDirExists(t, filepath.Join(cfg.BasePathAbsolute, "stacks"))
		})
	}
}

func TestEmptyImportIsRejected(t *testing.T) {
	cfg, renderer := fixture(t)
	_, err := Prepare(cfg, &Options{File: "stacks/deploy/dev.yaml", Imports: []string{"catalog/defaults", "   "}, Defaults: true}, renderer)
	require.ErrorIs(t, err, errUtils.ErrInvalidFlag)
	assert.Contains(t, err.Error(), "import cannot be empty")
}

func TestMultiDocumentStackManifestIsRejected(t *testing.T) {
	cfg, renderer := fixture(t)
	customFixture(t, customDefinition)
	write(t, "scaffolds/app/stack.yaml", "vars:\n  stage: one\n---\nvars:\n  stage: two\n")
	opts := &Options{File: "stacks/deploy/dev.yaml", Template: "application-stack", Defaults: true, Values: map[string]any{"environment": "dev"}}
	_, err := Prepare(cfg, opts, renderer)
	require.ErrorIs(t, err, errUtils.ErrInvalidConfig)
	assert.Contains(t, err.Error(), "generated stack stacks/deploy/dev.yaml: a stack manifest must contain one YAML document")
	assertNothingWritten(t, cfg, opts.File)
}

func TestValidateRenderedRequiresThePrimaryManifest(t *testing.T) {
	cfg, _ := fixture(t)
	// Rendered output that never produced the requested manifest cannot be accepted.
	files := []engine.File{{Path: filepath.Join("components", "terraform", "vpc", "main.tf"), Content: "# stub\n"}}
	_, err := validateRendered(cfg, cfg.BasePathAbsolute, files, filepath.Join("stacks", "deploy", "dev.yaml"))
	require.ErrorIs(t, err, errUtils.ErrInvalidConfig)
	assert.Contains(t, err.Error(), "scaffold did not generate stack manifest stacks/deploy/dev.yaml")
}

func TestKnownComponentKinds(t *testing.T) {
	require.NoError(t, custom.EnsureRegistered("createtestkind", "components/createtestkind"))
	commands := []schema.Command{{Name: "tools", Commands: []schema.Command{{Name: "deploy", Component: &schema.CommandComponent{Type: "nestedkind"}}}}}
	for kind, want := range map[string]bool{
		"terraform":       true,
		"helmfile":        true,
		"packer":          true,
		"createtestkind":  true, // Registered component provider.
		"nestedkind":      true, // Declared by a nested custom command.
		"unregisteredone": false,
		"":                false,
	} {
		assert.Equal(t, want, knownKind(kind, commands), "kind %q", kind)
	}
	assert.False(t, knownKind("nestedkind", nil), "without the custom command the kind is unknown")

	declared, err := parseComponents(&schema.AtmosConfiguration{Commands: commands}, []string{"nestedkind:job"})
	require.NoError(t, err)
	assert.Equal(t, []declaredComponent{{kind: "nestedkind", name: "job"}}, declared)
}

func TestApplyReportsStaleRecordRemovalFailure(t *testing.T) {
	root := t.TempDir()
	// The superseded record cannot be removed because it is a non-empty directory.
	stale := filepath.Join(recordDirectory, "stale.yaml")
	write(t, filepath.Join(root, stale, "child"), "occupied")
	plan := &Plan{
		Root:        root,
		Manifest:    filepath.Join("stacks", "dev.yaml"),
		Generation:  &generatorui.CreationPlan{Files: []engine.File{{Path: filepath.Join("stacks", "dev.yaml"), Content: "vars: {}\n"}}},
		staleRecord: stale,
	}

	err := plan.Apply(context.Background(), nil)
	require.ErrorIs(t, err, errUtils.ErrFileWrite)
	assert.Contains(t, err.Error(), filepath.ToSlash(stale))
	assert.NoFileExists(t, filepath.Join(root, "stacks", "dev.yaml"), "nothing is written after the record cannot be replaced")
}

func TestExistingStateReadFailuresAreReportedNotMistakenForAbsence(t *testing.T) {
	skipWithoutFileModeSupport(t)
	root := t.TempDir()
	// A regular file where a directory is expected makes the path impossible to inspect.
	write(t, filepath.Join(root, "stacks"), "file")
	require.ErrorIs(t, rejectExisting(root, filepath.Join("stacks", "dev.yaml")), errUtils.ErrReadFile)

	_, err := staleRecordPath(root, &engine.File{Path: filepath.Join("stacks", "record.yaml")})
	require.ErrorIs(t, err, errUtils.ErrReadFile)

	// Negative path: a genuinely absent path is not an error.
	require.NoError(t, rejectExisting(root, filepath.Join("missing", "dev.yaml")))
	path, err := staleRecordPath(root, &engine.File{Path: filepath.Join("missing", "record.yaml")})
	require.NoError(t, err)
	assert.Empty(t, path)
}

func TestDiscoverabilityPatternErrorsAreReported(t *testing.T) {
	root := t.TempDir()
	base := &schema.AtmosConfiguration{BasePathAbsolute: root, Stacks: schema.Stacks{BasePath: "stacks", IncludedPaths: []string{"deploy/**/*"}}}
	file := filepath.Join(root, "stacks", "deploy", "dev.yaml")

	// Without an absolute stacks path the base path and stacks.base_path locate the manifest.
	require.NoError(t, discoverable(base, file))

	included := *base
	included.Stacks.IncludedPaths = []string{"deploy/[unclosed"}
	require.Error(t, discoverable(&included, file))

	excluded := *base
	excluded.Stacks.IncludedPaths = []string{"deploy/**/*"}
	excluded.Stacks.ExcludedPaths = []string{"deploy/[unclosed"}
	require.Error(t, discoverable(&excluded, file))
}

func TestScaffoldConfigDirUsesTheDirectoryOfAConfigFile(t *testing.T) {
	root := t.TempDir()
	configDir := t.TempDir()
	write(t, filepath.Join(configDir, "atmos.yaml"), "base_path: .\n")

	dir, err := scaffoldConfigDir(&schema.AtmosConfiguration{CliConfigPath: filepath.Join(configDir, "atmos.yaml")}, root)
	require.NoError(t, err)
	assert.Equal(t, configDir, dir)
}

// recordingHook records each execution and optionally fails, so tests can observe hook timing and status.
type recordingHook struct {
	runnerstep.BaseHandler
	events *[]string
	fail   bool
}

func (h *recordingHook) Validate(*schema.WorkflowStep) error { return nil }

func (h *recordingHook) Execute(_ context.Context, step *schema.WorkflowStep, vars *runnerstep.Variables) (*runnerstep.StepResult, error) {
	content, err := vars.Resolve(step.Content)
	if err != nil {
		return nil, err
	}
	*h.events = append(*h.events, content)
	if h.fail {
		return nil, errUtils.ErrInvalidConfig
	}
	return runnerstep.NewStepResult(content), nil
}

func hookedDefinition(failingType, recordingType string) string {
	return strings.Split(customDefinition, "  hooks:\n")[0] + `  hooks:
    before:
      events: [before.scaffold.generate]
      kind: step
      type: ` + failingType + `
      with:
        content: 'before'
    after-failure:
      events: [after.scaffold.generate]
      when: failure
      kind: step
      type: ` + recordingType + `
      with:
        content: 'after-on-failure'
`
}

func TestFailingBeforeHookStopsApplyBeforeAnyWrite(t *testing.T) {
	cfg, renderer := fixture(t)
	events := []string{}
	runnerstep.Register(&recordingHook{BaseHandler: runnerstep.NewBaseHandler("stack-create-failing-before", runnerstep.CategoryOutput, false), events: &events, fail: true})
	runnerstep.Register(&recordingHook{BaseHandler: runnerstep.NewBaseHandler("stack-create-record-after-failed-hook", runnerstep.CategoryOutput, false), events: &events})
	customFixture(t, hookedDefinition("stack-create-failing-before", "stack-create-record-after-failed-hook"))
	plan, err := Prepare(cfg, &Options{File: "stacks/deploy/dev.yaml", Template: "application-stack", Values: map[string]any{"environment": "dev"}, Defaults: true}, renderer)
	require.NoError(t, err)

	err = plan.Apply(context.Background(), nil)
	require.ErrorIs(t, err, errUtils.ErrInvalidConfig)
	assert.Equal(t, []string{"before"}, events, "the failing hook ran and no after hook followed")
	assertNothingWritten(t, cfg, "stacks/deploy/dev.yaml")
}

func TestAfterHookObservesFailedApply(t *testing.T) {
	cfg, renderer := fixture(t)
	events := []string{}
	runnerstep.Register(&recordingHook{BaseHandler: runnerstep.NewBaseHandler("stack-create-passing-before", runnerstep.CategoryOutput, false), events: &events})
	runnerstep.Register(&recordingHook{BaseHandler: runnerstep.NewBaseHandler("stack-create-record-after-failed-apply", runnerstep.CategoryOutput, false), events: &events})
	customFixture(t, hookedDefinition("stack-create-passing-before", "stack-create-record-after-failed-apply"))
	plan, err := Prepare(cfg, &Options{File: "stacks/deploy/dev.yaml", Template: "application-stack", Values: map[string]any{"environment": "dev"}, Defaults: true}, renderer)
	require.NoError(t, err)
	// Make writing fail: the superseded record is a non-empty directory that cannot be removed.
	plan.staleRecord = filepath.Join(recordDirectory, "stale.yaml")
	write(t, filepath.Join(plan.Root, plan.staleRecord, "child"), "occupied")

	err = plan.Apply(context.Background(), nil)
	require.ErrorIs(t, err, errUtils.ErrFileWrite)
	assert.Equal(t, []string{"before", "after-on-failure"}, events, "the failure-only hook runs because the write failed")
	assert.NoFileExists(t, filepath.Join(plan.Root, "stacks", "deploy", "dev.yaml"))

	// Negative path: a successful apply never triggers the failure-only hook.
	events = events[:0]
	require.NoError(t, os.RemoveAll(filepath.Join(plan.Root, plan.staleRecord)))
	require.NoError(t, plan.Apply(context.Background(), nil))
	assert.Equal(t, []string{"before"}, events)
}

func TestBrokenConfigDirectoryFailsPlanning(t *testing.T) {
	cfg, renderer := fixture(t)
	cfg.CliConfigPath = "bad\x00path"
	_, err := Prepare(cfg, &Options{File: "stacks/deploy/dev.yaml", Defaults: true}, renderer)
	require.ErrorIs(t, err, errUtils.ErrInvalidConfig)
}

func TestUnreadableGenerationRecordLocationFailsPlanning(t *testing.T) {
	skipWithoutFileModeSupport(t)
	cfg, renderer := fixture(t)
	// A regular file where the generation record directory belongs hides whether a stale record exists.
	write(t, filepath.Join(cfg.BasePathAbsolute, ".atmos"), "file")
	_, err := Prepare(cfg, &Options{File: "stacks/deploy/dev.yaml", Name: "dev", Defaults: true}, renderer)
	require.ErrorIs(t, err, errUtils.ErrReadFile)
	assert.NoFileExists(t, filepath.Join(cfg.BasePathAbsolute, "stacks", "deploy", "dev.yaml"))
}

func TestStaleRecordRestoredOnFailureWithoutClobberingNewFile(t *testing.T) {
	for _, concurrent := range []bool{false, true} {
		t.Run(fmt.Sprint(concurrent), func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(recordDirectory, "record.yaml")
			write(t, filepath.Join(root, path), "old record")
			plan := &Plan{Root: root, staleRecord: path}
			finish, err := plan.stageStaleRecord()
			require.NoError(t, err)
			assert.NoFileExists(t, filepath.Join(root, path))
			if concurrent {
				write(t, filepath.Join(root, path), "concurrent record")
			}
			err = finish(false)
			content, readErr := os.ReadFile(filepath.Join(root, path))
			require.NoError(t, readErr)
			if concurrent {
				require.ErrorIs(t, err, errUtils.ErrFileWrite)
				assert.Contains(t, err.Error(), "retained backup")
				assert.Equal(t, "concurrent record", string(content))
				backups, globErr := filepath.Glob(filepath.Join(root, recordDirectory, ".stack-record-*"))
				require.NoError(t, globErr)
				require.Len(t, backups, 1)
				preserved, readErr := os.ReadFile(backups[0])
				require.NoError(t, readErr)
				assert.Equal(t, "old record", string(preserved))
			} else {
				require.NoError(t, err)
				assert.Equal(t, "old record", string(content))
			}
		})
	}
}

func TestLoadedEmptyCatalogDoesNotReadAmbientTemplates(t *testing.T) {
	config, _ := fixture(t)
	write(t, filepath.Join(config.BasePathAbsolute, "atmos.yaml"), "scaffold:\n  templates:\n    stack:\n      source: ./missing-template\n")
	config.ScaffoldConfigLoaded = true
	template, cleanup, err := resolveTemplate(config, &Options{}, config.BasePathAbsolute, "stacks/deploy/new.yaml")
	require.NoError(t, err)
	defer cleanup()
	assert.Equal(t, "embedded", template.Source)
}
