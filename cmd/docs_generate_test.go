package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestDocsGenerateCmd_Error(t *testing.T) {
	err := docsGenerateCmd.RunE(docsGenerateCmd, []string{})
	assert.Error(t, err, "docs generate command should return an error when called with no parameters")
}

// TestDocsGenerateCmd_AbsoluteBase verifies the command preserves an absolute module directory.
func TestDocsGenerateCmd_AbsoluteBase(t *testing.T) {
	_ = NewTestKit(t)
	workingDir, moduleDir := t.TempDir(), t.TempDir()
	t.Chdir(workingDir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workingDir)
	require.NoError(t, os.WriteFile(filepath.Join(moduleDir, "main.tf"), []byte(`variable "bucket" { type = string }`), 0o600))
	templatePath := filepath.Join(workingDir, "README.md.gomplate")
	require.NoError(t, os.WriteFile(templatePath, []byte(`{{ with terraform }}{{ range .Inputs }}{{ .Name }}{{ end }}{{ else }}unavailable{{ end }}`), 0o600))
	outputPath := filepath.Join(workingDir, "README.md")
	config := map[string]any{"docs": map[string]any{"generate": map[string]any{"absolute": map[string]any{
		"base-dir": moduleDir, "template": templatePath, "output": outputPath, "terraform": map[string]any{"enabled": true, "source": "."},
	}}}}
	content, err := yaml.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "atmos.yaml"), content, 0o600))
	require.NoError(t, docsGenerateCmd.RunE(docsGenerateCmd, []string{"absolute"}))
	actual, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.Equal(t, "bucket", string(actual))
}

// docsGenerateHookFixture writes an atmos.yaml with a docs hook per name that POSTs to server,
// and returns the output path.
func docsGenerateHookFixture(t *testing.T, serverURL string, names ...string) string {
	t.Helper()
	workingDir := t.TempDir()
	t.Chdir(workingDir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workingDir)
	t.Setenv("ATMOS_BASE_PATH", workingDir)
	outputPath := filepath.Join(workingDir, "README.md")
	hooks := map[string]any{}
	for _, name := range names {
		hooks[name] = map[string]any{
			"kind": "step", "type": "http", "events": []any{"after.docs.generate"},
			"with": map[string]any{"url": serverURL + "/" + name, "method": "POST"},
		}
	}
	config := map[string]any{"base_path": workingDir, "docs": map[string]any{"generate": map[string]any{"readme": map[string]any{
		"base-dir": workingDir, "output": outputPath, "hooks": hooks,
	}}}}
	content, err := yaml.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "atmos.yaml"), content, 0o600))
	return outputPath
}

func TestDocsGenerateCmd_SkipHooks(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  string
		want []string
	}{
		{name: "no flag runs every hook", want: []string{"/keep", "/skip"}},
		{name: "bare flag skips all", args: []string{"--skip-hooks"}, want: nil},
		{name: "named flag skips only that hook", args: []string{"--skip-hooks=skip"}, want: []string{"/keep"}},
		{name: "env var skips named hook", env: "skip", want: []string{"/keep"}},
		{name: "unrelated name skips nothing", args: []string{"--skip-hooks=other"}, want: []string{"/keep", "/skip"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t)
			var mu sync.Mutex
			var seen []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				seen = append(seen, r.URL.Path)
				mu.Unlock()
				w.WriteHeader(http.StatusNoContent)
			}))
			t.Cleanup(server.Close)
			outputPath := docsGenerateHookFixture(t, server.URL, "keep", "skip")
			t.Setenv("ATMOS_SKIP_HOOKS", tt.env)
			t.Cleanup(func() {
				_ = docsGenerateCmd.Flags().Set(skipHooksFlag, "")
				docsGenerateCmd.Flags().Lookup(skipHooksFlag).Changed = false
			})
			require.NoError(t, docsGenerateCmd.ParseFlags(tt.args))

			require.NoError(t, docsGenerateCmd.RunE(docsGenerateCmd, []string{"readme"}))

			assert.FileExists(t, outputPath, "skipping hooks never skips generation")
			mu.Lock()
			defer mu.Unlock()
			assert.ElementsMatch(t, tt.want, seen)
		})
	}
}

func TestDocsGenerateCmd_SkipHooksFlagRegistered(t *testing.T) {
	flag := docsGenerateCmd.Flags().Lookup(skipHooksFlag)
	require.NotNil(t, flag)
	assert.Equal(t, "*", flag.NoOptDefVal)
}
