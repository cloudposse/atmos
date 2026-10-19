package doctor

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	stackexec "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/validation"
)

// vtStackValidator records what the engine hands to the production stack validator.
type vtStackValidator struct {
	calls  int
	config *schema.AtmosConfiguration
	// inputs holds the decoded content of each file handed to the validator, keyed by its path.
	inputs map[string]map[string]any
	// files are the paths handed to the validator, in order.
	files  []string
	reader SourceReader
	err    error
	// onValidate runs inside Validate, before the configured error is returned.
	onValidate func()
}

func (v *vtStackValidator) Validate(_ context.Context, config *schema.AtmosConfiguration, inputs StackInputs) error {
	v.calls++
	v.config = config
	v.files = inputs.Files
	v.reader = inputs.Reader
	v.inputs = map[string]map[string]any{}
	for _, file := range inputs.Files {
		content, err := inputs.Reader.ReadFile(filepath.Join(config.StacksBaseAbsolutePath, filepath.FromSlash(file)))
		if err != nil {
			return err
		}
		var values map[string]any
		if err := yaml.Unmarshal(content, &values); err != nil {
			return err
		}
		v.inputs[file] = values
	}
	if v.onValidate != nil {
		v.onValidate()
	}
	return v.err
}

// vtProductionValidator adapts the production stack validator exactly as cmd/doctor wires it.
type vtProductionValidator struct{}

func (vtProductionValidator) Validate(ctx context.Context, config *schema.AtmosConfiguration, inputs StackInputs) error {
	return stackexec.ValidateStaticStacks(ctx, config, stackexec.StaticStackInputs{Files: inputs.Files, Reader: inputs.Reader})
}

func (vtProductionValidator) ValidateDependencies(ctx context.Context, config *schema.AtmosConfiguration, inputs StackInputs) error {
	return stackexec.ValidateStaticDependencies(ctx, config, stackexec.StaticStackInputs{Files: inputs.Files, Reader: inputs.Reader})
}

func (vtProductionValidator) DependencyTargets(ctx context.Context, config *schema.AtmosConfiguration, inputs StackInputs) (map[dependency.ComponentRef][]dependency.ComponentRef, error) {
	return stackexec.StaticDependencyTargets(ctx, config, stackexec.StaticStackInputs{Files: inputs.Files, Reader: inputs.Reader})
}

func vtStacksEngine(t *testing.T, v StackValidator) *Engine {
	t.Helper()
	e := testEngine(t)
	e.Stacks = v
	return e
}

func vtCheckByID(t *testing.T, r *Report, id string) Check {
	t.Helper()
	for _, c := range r.Checks {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("no %q check in %+v", id, r.Checks)
	return Check{}
}

func vtFindingsByRule(r *Report, id string) []Finding {
	var found []Finding
	for i := range r.Findings {
		if r.Findings[i].RuleID == id {
			found = append(found, r.Findings[i])
		}
	}
	return found
}

func TestStackValidationSkippedWithoutManifests(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	v := &vtStackValidator{}
	r, err := vtStacksEngine(t, v).Run(context.Background(), Options{Root: root, Offline: true, Only: []string{ruleStacks}})
	require.NoError(t, err)
	check := vtCheckByID(t, &r, ruleStacks)
	require.Equal(t, "skipped", check.Status)
	require.Equal(t, "No stack manifests", check.Reason)
	require.Zero(t, v.calls)
}

func TestStackValidationDisabledByOnlySelector(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "stacks/dev.yaml", "vars:\n  stage: dev\n")
	v := &vtStackValidator{}
	r, err := vtStacksEngine(t, v).Run(context.Background(), Options{Root: root, Offline: true, Only: []string{ruleEditorConfig}})
	require.NoError(t, err)
	require.Zero(t, v.calls)
	for _, c := range r.Checks {
		require.NotEqual(t, ruleStacks, c.ID)
	}
}

// Effective stack validation is only sound for a single, complete, static configuration.
func TestStackValidationIncompleteWhenInputsAreNotStatic(t *testing.T) {
	for _, tc := range []struct {
		name     string
		files    map[string]string
		profiles []string
		noEngine bool
	}{
		{name: "templated import path", files: map[string]string{"atmos.yaml": "{}\n", "stacks/catalog.yaml": "vars: {}\n", "stacks/dev.yaml": "import: ['{{ .catalog }}']\n"}},
		{name: "remote import", files: map[string]string{"atmos.yaml": "{}\n", "stacks/dev.yaml": "import: ['https://example.com/catalog.yaml']\n"}},
		{name: "import with context", files: map[string]string{"atmos.yaml": "{}\n", "stacks/catalog.yaml": "vars: {}\n", "stacks/dev.yaml": "import:\n  - path: catalog\n    context: {a: 1}\n"}},
		{name: "unresolved import", files: map[string]string{"atmos.yaml": "{}\n", "stacks/dev.yaml": "import: [absent]\n"}},
		{name: "template file import", files: map[string]string{"atmos.yaml": "{}\n", "stacks/catalog.yaml.tmpl": "vars: {}\n", "stacks/dev.yaml": "import: [catalog.yaml.tmpl]\n"}},
		{name: "template in an imported file", files: map[string]string{"atmos.yaml": "{}\n", "catalog/base.yaml": "vars:\n  x: '{{ .y }}'\n", "stacks/dev.yaml": "import: [../catalog/base]\n"}},
		{name: "templated stack", files: map[string]string{"atmos.yaml": "{}\n", "stacks/dev.yaml": "vars:\n  region: '{{ .region }}'\n"}},
		{name: "multiple configuration layers", files: map[string]string{"atmos.yaml": "{}\n", "atmos.d/extra.yaml": "{}\n", "stacks/dev.yaml": "vars: {}\n"}},
		{name: "profiles selected", files: map[string]string{"atmos.yaml": "{}\n", "stacks/dev.yaml": "vars: {}\n"}, profiles: []string{"ci"}},
		{name: "no validator injected", files: map[string]string{"atmos.yaml": "{}\n", "stacks/dev.yaml": "vars: {}\n"}, noEngine: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newProject(t)
			for path, content := range tc.files {
				projectFile(t, root, path, content)
			}
			v := &vtStackValidator{}
			e := vtStacksEngine(t, v)
			if tc.noEngine {
				e.Stacks = nil
			}
			r, err := e.Run(context.Background(), Options{Root: root, Offline: true, Profiles: tc.profiles, Only: []string{ruleStacks}})
			require.NoError(t, err)
			check := vtCheckByID(t, &r, ruleStacks)
			require.Equal(t, "incomplete", check.Status)
			require.Positive(t, check.Files)
			require.Contains(t, check.Reason, "incomplete because")
			require.Zero(t, v.calls, "an unverifiable project must never reach the stack validator")
			require.Empty(t, vtFindingsByRule(&r, ruleStacks))
		})
	}
}

func TestStackValidationReceivesStaticInputsAndConfig(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "base_path: infra\ncomponents:\n  terraform:\n    base_path: components/terraform\n  helmfile:\n    base_path: components/helmfile\n  packer:\n    base_path: components/packer\n  ansible:\n    base_path: components/ansible\n")
	projectFile(t, root, "infra/stacks/dev.yaml", "vars:\n  stage: dev\n")
	projectFile(t, root, "infra/stacks/prod/us.yaml", "vars:\n  stage: prod\ncomponents:\n  terraform:\n    app:\n      vars:\n        size: 3\n")
	v := &vtStackValidator{}
	r, err := vtStacksEngine(t, v).Run(context.Background(), Options{Root: root, Offline: true, Only: []string{ruleStacks}})
	require.NoError(t, err)
	require.Equal(t, 1, v.calls)
	require.Equal(t, map[string]map[string]any{
		"dev.yaml": {"vars": map[string]any{"stage": "dev"}},
		"prod/us.yaml": {
			"vars":       map[string]any{"stage": "prod"},
			"components": map[string]any{"terraform": map[string]any{"app": map[string]any{"vars": map[string]any{"size": 3}}}},
		},
	}, v.inputs, "inputs are keyed by stack-relative slash paths")

	base := filepath.Join(root, "infra")
	require.Equal(t, base, v.config.BasePath)
	require.Equal(t, filepath.Join(base, "stacks"), v.config.StacksBaseAbsolutePath)
	require.Equal(t, filepath.Join(base, "components", "terraform"), v.config.TerraformDirAbsolutePath)
	require.Equal(t, filepath.Join(base, "components", "helmfile"), v.config.HelmfileDirAbsolutePath)
	require.Equal(t, filepath.Join(base, "components", "packer"), v.config.PackerDirAbsolutePath)
	require.Equal(t, filepath.Join(base, "components", "ansible"), v.config.AnsibleDirAbsolutePath)

	check := vtCheckByID(t, &r, ruleStacks)
	require.Equal(t, statusPassed, check.Status)
	require.Equal(t, 2, check.Files)
	require.Empty(t, vtFindingsByRule(&r, ruleStacks))
}

func TestStackValidationKeepsAbsoluteComponentPaths(t *testing.T) {
	root := newProject(t)
	external := filepath.Join(newProject(t), "terraform")
	projectFile(t, root, "atmos.yaml", "components:\n  terraform:\n    base_path: '"+external+"'\n")
	projectFile(t, root, "stacks/dev.yaml", "vars: {}\n")
	v := &vtStackValidator{}
	_, err := vtStacksEngine(t, v).Run(context.Background(), Options{Root: root, Offline: true, Only: []string{ruleStacks}})
	require.NoError(t, err)
	require.Equal(t, external, v.config.TerraformDirAbsolutePath)
	require.Equal(t, root, v.config.BasePath)
}

// Stack discovery honors stacks.included_paths and stacks.excluded_paths with doublestar patterns.
func TestStackValidationHonorsIncludedAndExcludedPaths(t *testing.T) {
	for _, tc := range []struct {
		name, stacksConfig string
		want               []string
	}{
		{"no patterns include everything", "", []string{"catalog/base.yaml", "dev.yaml", "prod/us.yaml", "prod/skip.yaml"}},
		{"included only", "included_paths: ['dev.yaml', 'prod/**/*.yaml']", []string{"dev.yaml", "prod/us.yaml", "prod/skip.yaml"}},
		{"excluded only", "excluded_paths: ['catalog/**']", []string{"dev.yaml", "prod/us.yaml", "prod/skip.yaml"}},
		{"excluded wins over included", "included_paths: ['**/*.yaml']\n  excluded_paths: ['prod/skip.yaml', 'catalog/**']", []string{"dev.yaml", "prod/us.yaml"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", "stacks:\n  base_path: stacks\n  "+tc.stacksConfig+"\n")
			for _, path := range []string{"catalog/base.yaml", "dev.yaml", "prod/us.yaml", "prod/skip.yaml"} {
				projectFile(t, root, filepath.Join("stacks", filepath.FromSlash(path)), "vars:\n  file: "+path+"\n")
			}
			v := &vtStackValidator{}
			r, err := vtStacksEngine(t, v).Run(context.Background(), Options{Root: root, Offline: true, Only: []string{ruleStacks}})
			require.NoError(t, err)
			require.Equal(t, 1, v.calls)
			var got []string
			for path, values := range v.inputs {
				got = append(got, path)
				require.Equal(t, map[string]any{"file": path}, values["vars"], "each key carries its own manifest content")
			}
			require.ElementsMatch(t, tc.want, got)
			require.Equal(t, len(tc.want), vtCheckByID(t, &r, ruleStacks).Files)
		})
	}
}

func TestStackValidationRejectsMalformedPathPatterns(t *testing.T) {
	for _, field := range []string{"included_paths", "excluded_paths"} {
		t.Run(field, func(t *testing.T) {
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", "stacks:\n  "+field+": ['[']\n")
			projectFile(t, root, "stacks/dev.yaml", "vars: {}\n")
			v := &vtStackValidator{}
			r, err := vtStacksEngine(t, v).Run(context.Background(), Options{Root: root, Offline: true, Only: []string{ruleStacks}})
			require.ErrorIs(t, err, doublestar.ErrBadPattern)
			require.Zero(t, v.calls)
			require.NotEmpty(t, r.OperationalError)
		})
	}
}

func TestStackValidationFailureBecomesErrorFinding(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "stacks/dev.yaml", "vars: {}\n")
	var events []Event
	e := vtStacksEngine(t, &vtStackValidator{err: errors.New("duplicate component instance app in stack dev")})
	e.Observe = func(event Event) { events = append(events, event) }
	r, err := e.Run(context.Background(), Options{Root: root, Offline: true, Only: []string{ruleStacks}})
	require.NoError(t, err, "a failing stack is a finding, not an operational error")
	findings := vtFindingsByRule(&r, ruleStacks)
	require.Len(t, findings, 1)
	require.Equal(t, validation.SeverityError, findings[0].Severity)
	require.Equal(t, categoryValidation, findings[0].Category)
	require.Equal(t, "duplicate component instance app in stack dev", findings[0].Message)
	require.Equal(t, statusManual, findings[0].Status)
	require.False(t, findings[0].Fixable)
	require.Equal(t, 1, r.ExitCode("error"))
	check := vtCheckByID(t, &r, ruleStacks)
	require.Equal(t, "failed", check.Status)
	require.Equal(t, 1, check.Files)

	var sawDone bool
	for _, event := range events {
		if event.Label == "Static stacks" && event.Done {
			sawDone = true
			require.Equal(t, "failed", event.Status)
		}
	}
	require.True(t, sawDone, "completion event must report the check status")
}

func TestStackValidationCancellationPropagates(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "stacks/dev.yaml", "vars: {}\n")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	v := &vtStackValidator{err: context.Canceled, onValidate: cancel}
	r, err := vtStacksEngine(t, v).Run(ctx, Options{Root: root, Offline: true, Only: []string{ruleStacks}})
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, vtFindingsByRule(&r, ruleStacks), "cancellation is not a stack finding")
	for _, c := range r.Checks {
		require.NotEqual(t, ruleStacks, c.ID, "an interrupted check records no result")
	}
}

// The production validator is wired by cmd/doctor; this exercises the same engine
// path with real stack inheritance instead of a fake.
func TestStackValidationWithProductionInheritance(t *testing.T) {
	for _, tc := range []struct {
		name, stack, status string
	}{
		{"valid inheritance", "components:\n  terraform:\n    base:\n      metadata:\n        type: abstract\n    app:\n      metadata:\n        inherits: [base]\n", statusPassed},
		{"missing inherited component", "components:\n  terraform:\n    app:\n      metadata:\n        inherits: [missing]\n", "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", "{}\n")
			projectFile(t, root, "stacks/dev.yaml", tc.stack)
			e := testEngine(t)
			e.Stacks = vtProductionValidator{}
			r, err := e.Run(context.Background(), Options{Root: root, Offline: true, Only: []string{ruleStacks}})
			require.NoError(t, err)
			require.Equal(t, tc.status, vtCheckByID(t, &r, ruleStacks).Status)
			if tc.status == "failed" {
				require.NotEmpty(t, vtFindingsByRule(&r, ruleStacks))
			} else {
				require.Empty(t, vtFindingsByRule(&r, ruleStacks))
			}
		})
	}
}

// Stack validation reads only stack and configuration documents, so dynamic values elsewhere, such
// as a templated vendor ref or a workflow expression that starts with "!", must not switch it off.
// A dynamic value inside a stack still does.
func TestStackValidationIgnoresDynamicValuesItDoesNotRead(t *testing.T) {
	const stack = "vars:\n  stage: dev\ncomponents:\n  terraform:\n    app:\n      vars:\n        region: us-east-1\n"
	for _, tc := range []struct {
		name       string
		files      map[string]string
		wantStatus string
		wantCalls  int
	}{
		{
			name: "templated vendor ref and workflow expression",
			files: map[string]string{
				"stacks/dev.yaml":           stack,
				"vendor.yaml":               "spec:\n  sources:\n    - component: vpc\n      source: \"github.com/example/repo.git?ref={{.Version}}\"\n      version: 1.0.0\n",
				".github/workflows/ci.yaml": "on: push\njobs:\n  x:\n    if: '!cancelled()'\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ok\n",
			},
			wantStatus: statusPassed,
			wantCalls:  1,
		},
		{
			name:       "templated stack value",
			files:      map[string]string{"stacks/dev.yaml": stack + "        name: \"{{ .vars.stage }}\"\n"},
			wantStatus: "incomplete",
			wantCalls:  0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", "stacks:\n  base_path: stacks\n  included_paths: [\"**/*\"]\n")
			for path, content := range tc.files {
				projectFile(t, root, path, content)
			}
			v := &vtStackValidator{}
			r, err := vtStacksEngine(t, v).Run(context.Background(), Options{Root: root, Offline: true, Only: []string{ruleStacks}})
			require.NoError(t, err)
			require.Equal(t, tc.wantStatus, vtCheckByID(t, &r, ruleStacks).Status)
			require.Equal(t, tc.wantCalls, v.calls)
		})
	}
}

const (
	importedBase  = "components:\n  terraform:\n    base:\n      metadata:\n        type: abstract\n      vars:\n        size: small\n"
	importingDev  = "import:\n  - catalog/*\nvars:\n  stage: dev\ncomponents:\n  terraform:\n    app:\n      metadata:\n        inherits: [base]\n"
	importConfigs = "stacks:\n  base_path: stacks\n  included_paths: ['deploy/**/*']\n"
)

// A regular import no longer stops stack validation: the real loader merges it, so inheritance from
// an imported base resolves, and only the top-level manifests are handed to the validator.
func TestStackValidationFollowsRegularImports(t *testing.T) {
	for _, tc := range []struct {
		name, base, status string
	}{
		{"inherits from the imported base", importedBase, statusPassed},
		{"imported base lacks the component", "vars: {}\n", "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", importConfigs)
			projectFile(t, root, "stacks/catalog/base.yaml", tc.base)
			projectFile(t, root, "stacks/deploy/dev.yaml", importingDev)
			e := testEngine(t)
			e.Stacks = vtProductionValidator{}

			r, err := e.Run(context.Background(), Options{Root: root, Offline: true, Only: []string{ruleStacks}})

			require.NoError(t, err)
			check := vtCheckByID(t, &r, ruleStacks)
			require.Equal(t, tc.status, check.Status, "%+v", r)
			require.Equal(t, 1, check.Files, "only the included top-level manifest is a stack")
		})
	}
}

func TestStackValidatorReceivesTopLevelFilesAndAnOverlayReader(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", importConfigs)
	projectFile(t, root, "stacks/catalog/base.yaml", importedBase)
	projectFile(t, root, "stacks/deploy/dev.yaml", importingDev)
	v := &vtStackValidator{}

	_, err := vtStacksEngine(t, v).Run(context.Background(), Options{Root: root, Offline: true, Only: []string{ruleStacks}})

	require.NoError(t, err)
	require.Equal(t, []string{"deploy/dev.yaml"}, v.files)
	imported, err := v.reader.ReadFile(filepath.Join(root, "stacks", "catalog", "base.yaml"))
	require.NoError(t, err)
	require.Equal(t, importedBase, string(imported), "imports are served by the same reader")
}

// Imports that need evaluation or cannot be followed keep the check incomplete, and the reason
// says which declaration is responsible, with its file and line.
func TestStackBlockerNamesTheImportOrValueThatBlocks(t *testing.T) {
	for _, tc := range []struct {
		name   string
		files  map[string]string
		reason []string
	}{
		{"templated import path", map[string]string{"stacks/dev.yaml": "vars: {}\nimport: ['{{ .catalog }}']\n"}, []string{"stacks/dev.yaml:2", "import"}},
		{"remote import", map[string]string{"stacks/dev.yaml": "vars: {}\nimport:\n  - https://example.com/a.yaml\n"}, []string{"stacks/dev.yaml:3"}},
		{"import with context", map[string]string{"stacks/catalog.yaml": "vars: {}\n", "stacks/dev.yaml": "import:\n  - path: catalog\n    context: {a: 1}\n"}, []string{"Import with context", "stacks/dev.yaml:2"}},
		{"unresolved import", map[string]string{"stacks/dev.yaml": "\nimport: [absent]\n"}, []string{"Local import does not resolve: absent", "stacks/dev.yaml:2"}},
		{"template file import", map[string]string{"stacks/catalog.yaml.tmpl": "vars: {}\n", "stacks/dev.yaml": "import: [catalog.yaml.tmpl]\n"}, []string{"Template import not evaluated: stacks/catalog.yaml.tmpl", "stacks/dev.yaml:1"}},
		{"dynamic value in an imported file", map[string]string{"stacks/catalog/base.yaml": "vars:\n  a: 1\n  b: '{{ .x }}'\n", "stacks/dev.yaml": "import: [catalog/base]\n"}, []string{"Template expression", "stacks/catalog/base.yaml:3"}},
		{"yaml function in the stack", map[string]string{"stacks/dev.yaml": "vars:\n  a: !env HOME\n"}, []string{"YAML function !env", "stacks/dev.yaml:2"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", "{}\n")
			for path, content := range tc.files {
				projectFile(t, root, path, content)
			}
			v := &vtStackValidator{}

			r, err := vtStacksEngine(t, v).Run(context.Background(), Options{Root: root, Offline: true, Only: []string{ruleStacks}})

			require.NoError(t, err)
			check := vtCheckByID(t, &r, ruleStacks)
			require.Equal(t, "incomplete", check.Status)
			for _, want := range tc.reason {
				require.Contains(t, check.Reason, want)
			}
			require.NotContains(t, check.Reason, "static inputs and a stack validator", "the reason must be specific")
			require.Zero(t, v.calls)
		})
	}
}

func TestDependencyBlockerNamesTheInputThatBlocks(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "stacks/dev.yaml", "import: ['{{ .catalog }}']\ncomponents:\n  terraform:\n    app:\n      settings:\n        depends_on:\n          - component: database\n")

	r, err := vtStacksEngine(t, vtProductionValidator{}).Run(context.Background(), Options{Root: root, Offline: true, Fix: true, Only: []string{categoryDeprecations}})

	require.NoError(t, err)
	findings := vtFindingsByRule(&r, ruleComponentDependencies)
	require.Len(t, findings, 1)
	require.Equal(t, statusVerificationIncomplete, findings[0].Status)
	require.Contains(t, findings[0].Reason, "stacks/dev.yaml:1")
	require.NotContains(t, findings[0].Reason, "static inputs")
}
