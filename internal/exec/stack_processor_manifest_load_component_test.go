package exec

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hairyhenderson/gomplate/v3/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// manifestLoadTestTimeout bounds every describe in this file so a regression of the unbounded
// atmos.Component recursion fails the test quickly instead of hanging the whole test binary.
const manifestLoadTestTimeout = 30 * time.Second

// Compile-time sentinel: the manifest-load guard test constructs AtmosFuncs with this field.
var _ = AtmosFuncs{manifestLoadFile: "sentinel.yaml"}

// writeManifestLoadFixture creates a minimal Atmos project in a temp dir (two empty Terraform
// components named `base` and `app`) with a single stack manifest, and makes it the working dir.
func writeManifestLoadFixture(t *testing.T, stackFile, stackManifest string) {
	t.Helper()

	dir := t.TempDir()
	atmosYAML := `base_path: "./"
components:
  terraform:
    base_path: "components/terraform"
stacks:
  base_path: "stacks"
  included_paths: ["deploy/**/*"]
  name_template: "{{ .vars.stage }}"
templates:
  settings:
    enabled: true
`
	files := map[string]string{
		"atmos.yaml": atmosYAML,
		filepath.Join("components", "terraform", "base", "main.tf"): "",
		filepath.Join("components", "terraform", "app", "main.tf"):  "",
		filepath.Join("stacks", "deploy", stackFile):                stackManifest,
	}
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}

	t.Chdir(dir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")
	t.Setenv("ATMOS_BASE_PATH", "")

	ClearFindStacksMapCache()
	ClearLocalsExtractionCache()
	ClearFileContentCache()
	ClearBaseComponentConfigCache()
	ClearMergeContexts()
	ClearLastMergeContext()
	componentFuncSyncMap.Clear()
	t.Cleanup(func() {
		ClearFindStacksMapCache()
		ClearLocalsExtractionCache()
		componentFuncSyncMap.Clear()
	})
}

// describeWithDeadline runs `describe component` for the given component and stack and fails the
// test if it does not return within manifestLoadTestTimeout (the unbounded-recursion symptom).
func describeWithDeadline(t *testing.T, component, stack string) (map[string]any, error) {
	t.Helper()

	type result struct {
		sections map[string]any
		err      error
	}
	done := make(chan result, 1)
	go func() {
		sections, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
			Component:            component,
			Stack:                stack,
			ProcessTemplates:     true,
			ProcessYamlFunctions: true,
		})
		done <- result{sections, err}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), manifestLoadTestTimeout)
	defer cancel()
	select {
	case r := <-done:
		return r.sections, r.err
	case <-ctx.Done():
		t.Fatalf("describe component %q in stack %q did not return within %s (unbounded atmos.Component recursion)", component, stack, manifestLoadTestTimeout)
		return nil, nil
	}
}

// TestAtmosComponentInManifestWithLocals verifies that a stack manifest that has a `locals:` section
// and an `atmos.Component(...)` template call no longer recurses without limit: locals (and any other
// file-scoped context) make Atmos render the manifest while the stacks are still being loaded, and
// atmos.Component there used to describe the component, which loads every manifest again, and so on.
func TestAtmosComponentInManifestWithLocals(t *testing.T) {
	tests := []struct {
		name string
		// Stack and component names are unique per case because atmos.Component results are cached
		// process-wide by stack and component.
		stack     string
		component string
		manifest  string
		wantVarsY any
		wantErrIs error
		// wantErrContains lists substrings the error message must contain (chain naming).
		wantErrContains []string
	}{
		{
			name:      "locals and atmos.Component in component vars resolve",
			stack:     "mlA",
			component: "app",
			manifest: `locals:
  foo: bar
vars:
  stage: mlA
components:
  terraform:
    base:
      vars: {x: static}
    app:
      vars:
        y: '{{ (atmos.Component "base" "mlA").vars.x }}'
`,
			wantVarsY: "static",
		},
		{
			name:      "locals, a local reference, and atmos.Component resolve together",
			stack:     "mlB",
			component: "app",
			manifest: `locals:
  foo: bar
vars:
  stage: mlB
components:
  terraform:
    base:
      vars: {x: static}
    app:
      vars:
        y: '{{ (atmos.Component "base" "mlB").vars.x }}-{{ .locals.foo }}'
`,
			wantVarsY: "static-bar",
		},
		{
			name:      "control without locals still resolves",
			stack:     "mlC",
			component: "app",
			manifest: `vars:
  stage: mlC
components:
  terraform:
    base:
      vars: {x: static}
    app:
      vars:
        y: '{{ (atmos.Component "base" "mlC").vars.x }}'
`,
			wantVarsY: "static",
		},
		{
			name:      "locals and atmos.Component in top-level vars resolve",
			stack:     "mlD",
			component: "app",
			manifest: `locals:
  foo: bar
vars:
  stage: mlD
  y: '{{ (atmos.Component "base" "mlD").vars.x }}'
components:
  terraform:
    base:
      vars: {x: static, y: own}
    app: {}
`,
			wantVarsY: "static",
		},
		{
			name:      "locals and atmos.Component with a local as argument resolve",
			stack:     "mlH",
			component: "app",
			manifest: `locals:
  target: base
vars:
  stage: mlH
components:
  terraform:
    base:
      vars: {x: static}
    app:
      vars:
        y: '{{ (atmos.Component .locals.target "mlH").vars.x }}'
`,
			wantVarsY: "static",
		},
		{
			name:      "locals and atmos.Component in a control structure defer to the component render",
			stack:     "mlI",
			component: "app",
			manifest: `locals:
  foo: bar
vars:
  stage: mlI
components:
  terraform:
    base:
      vars: {x: static}
    app:
      vars:
        y: '{{ if (atmos.Component "base" "mlI").vars.x }}yes{{ else }}no{{ end }}'
`,
			wantVarsY: "yes",
		},
		{
			name:      "locals and a component referencing itself is a clean cycle error",
			stack:     "mlE",
			component: "app",
			manifest: `locals:
  foo: bar
vars:
  stage: mlE
components:
  terraform:
    app:
      vars:
        y: '{{ (atmos.Component "app" "mlE").vars.y }}'
`,
			wantErrIs:       errUtils.ErrCircularDependency,
			wantErrContains: []string{"Component 'app' in stack 'mlE'"},
		},
		{
			name:      "locals and a two-component cycle is a clean cycle error",
			stack:     "mlF",
			component: "app",
			manifest: `locals:
  foo: bar
vars:
  stage: mlF
components:
  terraform:
    base:
      vars:
        x: '{{ (atmos.Component "app" "mlF").vars.y }}'
    app:
      vars:
        y: '{{ (atmos.Component "base" "mlF").vars.x }}'
`,
			wantErrIs:       errUtils.ErrCircularDependency,
			wantErrContains: []string{"Component 'app' in stack 'mlF'", "Component 'base' in stack 'mlF'"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeManifestLoadFixture(t, tt.stack+".yaml", tt.manifest)

			sections, err := describeWithDeadline(t, tt.component, tt.stack)

			if tt.wantErrIs != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErrIs)
				for _, want := range tt.wantErrContains {
					assert.Contains(t, fmt.Sprint(err), want)
				}
				return
			}

			require.NoError(t, err)
			vars, ok := sections["vars"].(map[string]any)
			require.True(t, ok, "vars section must be a map")
			assert.Equal(t, tt.wantVarsY, vars["y"])
		})
	}
}

// TestAtmosComponentInEagerlyRenderedManifest covers a manifest that Atmos renders in full while
// loading (a `.yaml.tmpl` file): a plain atmos.Component action is left for the component render and
// resolves, while a form that cannot be deferred (a control structure) fails with a clear error naming
// the file instead of recursing without limit or surfacing a confusing YAML parse error.
func TestAtmosComponentInEagerlyRenderedManifest(t *testing.T) {
	tests := []struct {
		name      string
		stack     string
		varsY     string
		wantVarsY any
		wantErrIs error
	}{
		{
			name:      "plain action resolves",
			stack:     "mlJ",
			varsY:     `'{{ (atmos.Component "base" "mlJ").vars.x }}'`,
			wantVarsY: "static",
		},
		{
			name:      "control structure fails with a clear error",
			stack:     "mlK",
			varsY:     `'{{ if (atmos.Component "base" "mlK").vars.x }}yes{{ else }}no{{ end }}'`,
			wantErrIs: errUtils.ErrComponentFuncDuringManifestLoad,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeManifestLoadFixture(t, tt.stack+".yaml.tmpl", fmt.Sprintf(`vars:
  stage: %s
components:
  terraform:
    base:
      vars: {x: static}
    app:
      vars:
        y: %s
`, tt.stack, tt.varsY))

			sections, err := describeWithDeadline(t, "app", tt.stack)

			if tt.wantErrIs != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErrIs)
				assert.Contains(t, fmt.Sprint(err), tt.stack+".yaml.tmpl")
				return
			}

			require.NoError(t, err)
			vars, ok := sections["vars"].(map[string]any)
			require.True(t, ok, "vars section must be a map")
			assert.Equal(t, tt.wantVarsY, vars["y"])
		})
	}
}

// TestAtmosFuncs_Component_DuringManifestLoad verifies the re-entry guard directly: while a stack
// manifest is being loaded, atmos.Component must not describe the target (which would load every
// manifest again, including the one being rendered) and must return a sentinel error that names the
// manifest and the call. When no manifest is being loaded the normal path is used (negative path).
func TestAtmosFuncs_Component_DuringManifestLoad(t *testing.T) {
	newFuncs := func(manifestFile string) *AtmosFuncs {
		return &AtmosFuncs{
			atmosConfig:         &schema.AtmosConfiguration{},
			configAndStacksInfo: &schema.ConfigAndStacksInfo{},
			ctx:                 context.Background(),
			gomplateData:        &data.Data{},
			manifestLoadFile:    manifestFile,
		}
	}

	t.Run("guard returns sentinel error naming file and call", func(t *testing.T) {
		_, err := newFuncs("deploy/prod.yaml").Component("vpc", "prod")

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrComponentFuncDuringManifestLoad)
		assert.Contains(t, err.Error(), "deploy/prod.yaml")
		assert.Contains(t, err.Error(), `atmos.Component("vpc", "prod")`)
	})

	t.Run("no guard outside manifest load", func(t *testing.T) {
		_, err := newFuncs("").Component("", "")

		require.Error(t, err)
		assert.NotErrorIs(t, err, errUtils.ErrComponentFuncDuringManifestLoad)
	})
}

// TestProcessTmpl_ManifestLoad verifies the manifest-load renderer: ordinary templates keep rendering,
// atmos.Component actions are emitted as template text (with locals resolved), and forms that cannot be
// deferred fail with the sentinel error.
func TestProcessTmpl_ManifestLoad(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tmplCtx := map[string]any{
		"locals": map[string]any{"foo": "bar", "n": 3},
		"vars":   map[string]any{"stage": "dev"},
	}

	tests := []struct {
		name      string
		input     string
		want      string
		wantErrIs error
	}{
		{
			name:  "ordinary template renders",
			input: "name: {{ .locals.foo }}",
			want:  "name: bar",
		},
		{
			name:  "atmos.Component is emitted as template text",
			input: `y: {{ (atmos.Component "a" "b").vars.x }}`,
			want:  `y: {{(atmos.Component "a" "b").vars.x}}`,
		},
		{
			name:  "locals in arguments are resolved, vars are left for the component render",
			input: `y: {{ (atmos.Component .locals.foo .vars.stage).vars.x }} {{ .locals.n }}`,
			want:  `y: {{(atmos.Component "bar" .vars.stage).vars.x}} 3`,
		},
		{
			name:      "control structure cannot be deferred",
			input:     `{{ if (atmos.Component "a" "b").vars.x }}y{{ end }}`,
			wantErrIs: errUtils.ErrComponentFuncDuringManifestLoad,
		},
		{
			name:      "variable declaration cannot be deferred",
			input:     `{{ $c := atmos.Component "a" "b" }}{{ $c.vars.x }}`,
			wantErrIs: errUtils.ErrComponentFuncDuringManifestLoad,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := processTmpl(atmosConfig, tmplInput{name: "deploy/x.yaml", value: tt.input, data: tmplCtx, manifestLoadFile: "deploy/x.yaml"})

			if tt.wantErrIs != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErrIs)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	t.Run("regular rendering still evaluates atmos.Component", func(t *testing.T) {
		_, err := ProcessTmpl(atmosConfig, "x", `{{ atmos.Component "" "" }}`, tmplCtx, false)

		require.Error(t, err)
		assert.NotErrorIs(t, err, errUtils.ErrComponentFuncDuringManifestLoad)
	})
}
