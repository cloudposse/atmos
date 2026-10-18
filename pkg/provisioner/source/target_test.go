package source

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels for the schema fields these tests configure.
var (
	_ = schema.Terraform{BasePath: ""}
	_ = schema.AwsCloudFormation{BasePath: ""}
)

// componentTypeFixture configures one component type's base path under root.
type componentTypeFixture struct {
	componentType string
	dirName       string
}

func componentTypeFixtures() []componentTypeFixture {
	return []componentTypeFixture{
		{componentType: "terraform", dirName: "terraform"},
		{componentType: "aws/cloudformation", dirName: "cloudformation"},
	}
}

func (f componentTypeFixture) config(root string) *schema.AtmosConfiguration {
	config := &schema.AtmosConfiguration{BasePath: root}
	switch f.componentType {
	case "terraform":
		config.Components.Terraform.BasePath = filepath.Join("components", f.dirName)
	default:
		config.Components.CloudFormation.BasePath = filepath.Join("components", f.dirName)
	}
	return config
}

func (f componentTypeFixture) componentDir(root, name string) string {
	return filepath.Join(root, "components", f.dirName, name)
}

// localSourceDir creates a directory source containing a template file.
func localSourceDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "template.yaml"), []byte("Resources: {}\n"), 0o600))
	return dir
}

func TestResolveTarget(t *testing.T) {
	for _, fx := range componentTypeFixtures() {
		t.Run(fx.componentType, func(t *testing.T) {
			root := t.TempDir()
			config := fx.config(root)

			t.Run("instance name differs from metadata.component", func(t *testing.T) {
				target, err := ResolveTarget(config, fx.componentType, "instance", map[string]any{
					"atmos_component": "instance",
					"metadata":        map[string]any{"component": "base"},
				})
				require.NoError(t, err)
				assert.Equal(t, fx.componentDir(root, "base"), target.Dir)
				assert.Equal(t, "base", target.Component)
				assert.False(t, target.IsWorkdir)
			})

			t.Run("component key wins over metadata.component", func(t *testing.T) {
				target, err := ResolveTarget(config, fx.componentType, "instance", map[string]any{
					"component": "from-component",
					"metadata":  map[string]any{"component": "from-metadata"},
				})
				require.NoError(t, err)
				assert.Equal(t, fx.componentDir(root, "from-component"), target.Dir)
			})

			t.Run("fallback name used when configuration has none", func(t *testing.T) {
				target, err := ResolveTarget(config, fx.componentType, "fallback", map[string]any{})
				require.NoError(t, err)
				assert.Equal(t, fx.componentDir(root, "fallback"), target.Dir)
			})

			t.Run("no name at all", func(t *testing.T) {
				_, err := ResolveTarget(config, fx.componentType, "", map[string]any{})
				require.ErrorIs(t, err, errUtils.ErrSourceProvision)
			})

			t.Run("working_directory override", func(t *testing.T) {
				override := filepath.Join(root, "elsewhere")
				target, err := ResolveTarget(config, fx.componentType, "instance", map[string]any{
					"metadata": map[string]any{"component": "base", "working_directory": override},
				})
				require.NoError(t, err)
				assert.Equal(t, override, target.Dir)
			})

			t.Run("workdir enabled", func(t *testing.T) {
				section := map[string]any{
					"atmos_component": "instance",
					"atmos_stack":     "dev",
					"metadata":        map[string]any{"component": "base"},
					"provision":       map[string]any{"workdir": map[string]any{"enabled": true}},
				}
				target, err := ResolveTarget(config, fx.componentType, "instance", section)
				require.NoError(t, err)
				want, err := workdir.BuildPath(root, fx.componentType, "base", "dev", section)
				require.NoError(t, err)
				assert.Equal(t, want, target.Dir)
				assert.True(t, target.IsWorkdir)
			})

			t.Run("workdir enabled without stack", func(t *testing.T) {
				_, err := ResolveTarget(config, fx.componentType, "instance", map[string]any{
					"atmos_component": "instance",
					"provision":       map[string]any{"workdir": map[string]any{"enabled": true}},
				})
				require.ErrorIs(t, err, errUtils.ErrSourceProvision)
			})
		})
	}
}

func TestResolveTarget_RejectsUnsafeComponentNames(t *testing.T) {
	cases := []struct {
		name      string
		component string
		wantErr   bool
	}{
		{name: "unix absolute", component: "/Users/me/outside/mc-abs", wantErr: true},
		{name: "backslash rooted", component: `\share\component`, wantErr: true},
		{name: "windows drive backslash", component: `C:\components\vpc`, wantErr: true},
		{name: "windows drive slash", component: "c:/components/vpc", wantErr: true},
		{name: "windows drive relative", component: `D:vpc`, wantErr: true},
		{name: "parent escape", component: "../../escape", wantErr: true},
		{name: "parent escape backslash", component: `..\..\escape`, wantErr: true},
		{name: "parent escape after nesting", component: "a/../../escape", wantErr: true},
		{name: "bare parent", component: "..", wantErr: true},
		{name: "nested", component: "networking/vpc"},
		{name: "dot segment collapses inside", component: "a/../b"},
		{name: "plain", component: "vpc"},
	}
	for _, fx := range componentTypeFixtures() {
		for _, tt := range cases {
			for _, workdirEnabled := range []bool{false, true} {
				t.Run(fx.componentType+"/"+tt.name+map[bool]string{false: "", true: "/workdir"}[workdirEnabled], func(t *testing.T) {
					root := t.TempDir()
					section := map[string]any{"metadata": map[string]any{"component": tt.component}, "atmos_stack": "dev", "atmos_component": "instance"}
					if workdirEnabled {
						section["provision"] = map[string]any{"workdir": map[string]any{"enabled": true}}
					}
					_, err := ResolveTarget(fx.config(root), fx.componentType, "instance", section)
					if tt.wantErr {
						require.ErrorIs(t, err, errUtils.ErrSourceComponentNameInvalid)
						assert.Contains(t, err.Error(), tt.component, "the error names the offending component")
						assert.NoDirExists(t, filepath.Join(root, "components", fx.dirName, "Users"), "nothing is created")
					} else {
						require.NoError(t, err)
					}
				})
			}
		}
	}
}

// TestProvisionAndRuntimeAgreeOnTarget proves `source pull` (Provision) and the runtime
// (AutoProvisionSource) provision into the same directory, and both leave provenance behind.
func TestProvisionAndRuntimeAgreeOnTarget(t *testing.T) {
	for _, fx := range componentTypeFixtures() {
		for _, workdirEnabled := range []bool{false, true} {
			name := fx.componentType + map[bool]string{false: "/shared", true: "/workdir"}[workdirEnabled]
			t.Run(name, func(t *testing.T) {
				newSection := func() map[string]any {
					section := map[string]any{
						"source":          map[string]any{"uri": localSourceDir(t)},
						"atmos_component": "instance",
						"atmos_stack":     "dev",
						"metadata":        map[string]any{"component": "base"},
					}
					if workdirEnabled {
						section["provision"] = map[string]any{"workdir": map[string]any{"enabled": true}}
					}
					return section
				}

				pullRoot := t.TempDir()
				pullSection := newSection()
				require.NoError(t, Provision(t.Context(), &ProvisionParams{
					AtmosConfig: fx.config(pullRoot), ComponentType: fx.componentType, Component: "instance",
					Stack: "dev", ComponentConfig: pullSection,
				}))
				pullTarget, err := ResolveTarget(fx.config(pullRoot), fx.componentType, "instance", pullSection)
				require.NoError(t, err)
				assert.FileExists(t, filepath.Join(pullTarget.Dir, "template.yaml"))
				assert.True(t, HasProvenance(pullTarget.Dir))
				assert.NoDirExists(t, fx.componentDir(pullRoot, "instance"), "the instance-named directory is never created")

				runtimeRoot := t.TempDir()
				runtimeSection := newSection()
				require.NoError(t, AutoProvisionSource(t.Context(), fx.config(runtimeRoot), fx.componentType, runtimeSection, nil, provisioner.OutputWriters{}))
				runtimeTarget, err := ResolveTarget(fx.config(runtimeRoot), fx.componentType, "instance", runtimeSection)
				require.NoError(t, err)
				assert.FileExists(t, filepath.Join(runtimeTarget.Dir, "template.yaml"))
				assert.True(t, HasProvenance(runtimeTarget.Dir))

				rel := func(root, dir string) string {
					r, err := filepath.Rel(root, dir)
					require.NoError(t, err)
					return r
				}
				assert.Equal(t, rel(pullRoot, pullTarget.Dir), rel(runtimeRoot, runtimeTarget.Dir), "pull and runtime use the same relative location")
			})
		}
	}
}

// TestProvision_FailureReportsSentinelOnce guards against the doubled "source provisioning failed:"
// prefix produced when an already-wrapped download error was wrapped again.
func TestProvision_FailureReportsSentinelOnce(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	t.Cleanup(server.Close)

	for _, fx := range componentTypeFixtures() {
		t.Run(fx.componentType, func(t *testing.T) {
			root := t.TempDir()
			err := Provision(t.Context(), &ProvisionParams{
				AtmosConfig: fx.config(root), ComponentType: fx.componentType, Component: "vpc", Stack: "dev",
				ComponentConfig: map[string]any{
					"source":          map[string]any{"uri": server.URL + "/missing.tar.gz", "retry": map[string]any{"max_attempts": 1}},
					"atmos_component": "vpc",
				},
			})
			require.ErrorIs(t, err, errUtils.ErrSourceProvision)
			assert.Equal(t, 1, strings.Count(err.Error(), errUtils.ErrSourceProvision.Error()), "sentinel text appears once: %s", err)
			assert.NoDirExists(t, fx.componentDir(root, "vpc")+"/.atmos", "no provenance for a failed provision")
		})
	}
}
