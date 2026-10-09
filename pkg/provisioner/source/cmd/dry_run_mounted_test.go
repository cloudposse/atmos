package cmd_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfncmd "github.com/cloudposse/atmos/cmd/aws/cloudformation"
	"github.com/cloudposse/atmos/pkg/provisioner/source"
)

func TestCloudFormationSourceDryRun_MountedCommandsPreserveSource(t *testing.T) {
	for _, tt := range []struct {
		verb  string
		force bool
		env   bool
	}{{"pull", true, false}, {"delete", true, false}, {"delete", false, false}, {"pull", true, true}, {"delete", true, true}, {"delete", false, true}} {
		t.Run(tt.verb+map[bool]string{true: "/forced", false: "/confirmation"}[tt.force]+map[bool]string{true: "/env", false: "/cli"}[tt.env], func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			t.Cleanup(server.Close)
			rootDir := t.TempDir()
			componentDir := filepath.Join(rootDir, "components", "cloudformation", "vpc")
			require.NoError(t, os.MkdirAll(componentDir, 0o755))
			template := filepath.Join(componentDir, "template.yaml")
			require.NoError(t, os.WriteFile(template, []byte("Resources: {}\n"), 0o600))
			// `source delete` only acts on directories the provisioner created.
			require.NoError(t, source.WriteProvenance(componentDir, &source.Provenance{Component: "vpc", Source: "https://example.invalid/component.zip"}))
			writeSourceDryRunFixture(t, rootDir, server.URL)
			t.Setenv("ATMOS_CLI_CONFIG_PATH", rootDir)
			t.Setenv("ATMOS_BASE_PATH", rootDir)
			t.Setenv("ATMOS_INTERACTIVE", "false")

			root := cfncmd.CloudFormationCmd
			cmd, _, err := root.Find([]string{"source", tt.verb})
			require.NoError(t, err)
			require.Equal(t, tt.verb, cmd.Name())
			require.NotNil(t, root.PersistentFlags().Lookup("dry-run"))
			require.Nil(t, cmd.LocalNonPersistentFlags().Lookup("dry-run"), "source inherits the flag from CloudFormation")
			for _, name := range []string{"dry-run", "stack", "force"} {
				flag := cmd.Flag(name)
				require.NotNil(t, flag)
				previous, changed := flag.Value.String(), flag.Changed
				t.Cleanup(func() { require.NoError(t, flag.Value.Set(previous)); flag.Changed = changed })
			}
			t.Cleanup(func() { root.SetArgs(nil) })
			t.Setenv("ATMOS_DRY_RUN", "false")
			args := []string{"source", tt.verb, "vpc", "--stack", "dev"}
			if tt.env {
				t.Setenv("ATMOS_DRY_RUN", "true")
			} else {
				args = append(args, "--dry-run")
			}
			if tt.force {
				args = append(args, "--force")
			}
			root.SetArgs(args)
			require.NoError(t, root.Execute())
			assert.Zero(t, requests.Load(), "dry-run must not download source")
			contents, err := os.ReadFile(template)
			require.NoError(t, err)
			assert.Equal(t, "Resources: {}\n", string(contents))
			entries, err := os.ReadDir(componentDir)
			require.NoError(t, err)
			names := make([]string, 0, len(entries))
			for _, entry := range entries {
				names = append(names, entry.Name())
			}
			assert.ElementsMatch(t, []string{".atmos", "template.yaml"}, names)
		})
	}
}

func writeSourceDryRunFixture(t *testing.T, rootDir, sourceURL string) {
	t.Helper()
	files := map[string]any{
		"atmos.yaml":                        map[string]any{"base_path": rootDir, "components": map[string]any{"aws/cloudformation": map[string]any{"base_path": "components/cloudformation"}}, "stacks": map[string]any{"base_path": "stacks", "included_paths": []string{"**/*"}, "name_template": "{{ .vars.stage }}"}},
		filepath.Join("stacks", "dev.yaml"): map[string]any{"vars": map[string]any{"stage": "dev"}, "components": map[string]any{"aws/cloudformation": map[string]any{"vpc": map[string]any{"source": map[string]any{"uri": sourceURL + "/component.zip"}}}}},
	}
	for name, value := range files {
		path := filepath.Join(rootDir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		content, err := json.Marshal(value)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, content, 0o600))
	}
}
