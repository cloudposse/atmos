package exec

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDescribeStacksStaticIgnoresEvaluatedCache(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "dev.yaml")
	require.NoError(t, os.WriteFile(file, []byte("name: dev\ncomponents:\n  terraform:\n    app:\n      backend_type: local\n      backend:\n        local:\n          path: '{{ external.path }}'\n"), 0o600))
	config := &schema.AtmosConfiguration{StacksBaseAbsolutePath: root, StackConfigFilesAbsolutePaths: []string{file}}
	key := getFindStacksMapCacheKey(config, false)
	poison := map[string]any{"cached": "evaluated-before-inspection"}
	findStacksMapCacheMu.Lock()
	old, existed := findStacksMapCache[key]
	findStacksMapCache[key] = &findStacksMapCacheEntry{stacksMap: poison}
	findStacksMapCacheMu.Unlock()
	t.Cleanup(func() {
		findStacksMapCacheMu.Lock()
		defer findStacksMapCacheMu.Unlock()
		if existed {
			findStacksMapCache[key] = old
		} else {
			delete(findStacksMapCache, key)
		}
	})
	result, err := ExecuteDescribeStacksWithOptions(config, "dev", nil, []string{"terraform"}, nil,
		false, true, true, true, nil, nil, false, DescribeStacksErrorOptions{StaticOnly: true})
	require.NoError(t, err)
	stack := result["dev"].(map[string]any)
	component := stack["components"].(map[string]any)["terraform"].(map[string]any)["app"].(map[string]any)
	require.Equal(t, "{{ external.path }}", component["backend"].(map[string]any)["path"])
	require.NotContains(t, component, "workspace")
	findStacksMapCacheMu.RLock()
	cached := findStacksMapCache[key].stacksMap
	findStacksMapCacheMu.RUnlock()
	require.Equal(t, poison, cached, "static discovery must not overwrite another caller's evaluated cache")
}

func TestDescribeStacksStaticSkipsLocalsAndImportTemplates(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "dev.yaml")
	importFile := filepath.Join(root, "base.yaml.tmpl")
	source := "name: dev\nlocals:\n  forbidden: !exec atmos-static-must-not-execute\nimport: [base.yaml.tmpl]\n"
	imported := `locals:
  forbidden: !exec atmos-static-import-must-not-execute
components:
  terraform:
    app:
      backend_type: local
      backend:
        local:
          path: '{{ file.Read "must-not-read" }}'
`
	require.NoError(t, os.WriteFile(file, []byte(source), 0o600))
	require.NoError(t, os.WriteFile(importFile, []byte(imported), 0o600))
	key := localsCacheKey(importFile, imported) + "\x00edition=\x00stack-literals"
	poison := &extractLocalsResult{hasLocals: true, locals: map[string]any{"cached": "evaluated"}}
	localsExtractionCache.Store(key, poison)
	t.Cleanup(func() { localsExtractionCache.Delete(key) })
	config := &schema.AtmosConfiguration{StacksBaseAbsolutePath: root, StackConfigFilesAbsolutePaths: []string{file}}
	result, err := ExecuteDescribeStacksWithOptions(config, "dev", nil, []string{"terraform"}, nil,
		false, false, false, true, nil, nil, false, DescribeStacksErrorOptions{StaticOnly: true})
	require.NoError(t, err)
	component := result["dev"].(map[string]any)["components"].(map[string]any)["terraform"].(map[string]any)["app"].(map[string]any)
	require.Equal(t, `{{ file.Read "must-not-read" }}`, component["backend"].(map[string]any)["path"])
	require.Nil(t, component["locals"], "evaluated locals cache must not be injected")
	cached, exists := localsExtractionCache.Load(key)
	require.True(t, exists)
	require.Same(t, poison, cached)
}

func TestDescribeStacksStaticRejectsRemoteImport(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "dev.yaml")
	require.NoError(t, os.WriteFile(file, []byte("name: dev\nimport: [https://must-not-fetch.invalid/stack.yaml]\n"), 0o600))
	config := &schema.AtmosConfiguration{StacksBaseAbsolutePath: root, StackConfigFilesAbsolutePaths: []string{file}}
	_, err := ExecuteDescribeStacksWithOptions(config, "dev", nil, nil, nil,
		false, false, false, true, nil, nil, false, DescribeStacksErrorOptions{StaticOnly: true})
	require.ErrorIs(t, err, errUtils.ErrInvalidImport)
	require.Contains(t, err.Error(), "without fetching it")
}

func TestDescribeStacksStaticRejectsTemplatedImport(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte("vars: {remote: unexpected}\n"))
	}))
	t.Cleanup(server.Close)
	cases := []struct {
		name        string
		delimiters  []string
		path        string
		skipMissing bool
		wantErr     error
	}{
		{"default optional", nil, `{{ exec.Command "touch" "MARKER" }}`, true, errUtils.ErrInvalidImport},
		{"custom optional", []string{"[[", "]]"}, `[[ .vars.import ]]`, true, errUtils.ErrInvalidImport},
		{"custom executable", []string{"[[", "]]"}, `[[ exec.Command "touch" "MARKER" ]]`, true, errUtils.ErrInvalidImport},
		{"custom required", []string{"[[", "]]"}, `[[ .vars.import ]]`, false, errUtils.ErrInvalidImport},
		{"custom remote optional", []string{"[[", "]]"}, server.URL + "/[[ .vars.import ]]", true, errUtils.ErrInvalidImport},
		{"default literal optional", nil, "missing", true, nil},
		{"custom literal optional", []string{"[[", "]]"}, "missing", true, nil},
		{"default braces are literal with custom delimiters", []string{"[[", "]]"}, "{{ .vars.import }}", true, nil},
		{"invalid delimiter count", []string{"[["}, "missing", true, errUtils.ErrInvalidTemplateSettings},
		{"empty delimiter", []string{"", "]]"}, "missing", true, errUtils.ErrInvalidTemplateSettings},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			file := filepath.Join(root, "dev.yaml")
			marker := filepath.Join(root, "executed")
			importPath := strings.ReplaceAll(tc.path, "MARKER", filepath.ToSlash(marker))
			source := fmt.Sprintf(`name: dev
locals:
  forbidden: !exec %q
import:
  - path: %q
    skip_if_missing: %t
components:
  terraform:
    app:
      vars:
        stage: dev
`, "touch "+marker, importPath, tc.skipMissing)
			require.NoError(t, os.WriteFile(file, []byte(source), 0o600))
			config := &schema.AtmosConfiguration{StacksBaseAbsolutePath: root, StackConfigFilesAbsolutePaths: []string{file}}
			config.Templates.Settings.Enabled = true
			config.Templates.Settings.Delimiters = tc.delimiters
			result, err := ExecuteDescribeStacksWithOptions(config, "dev", nil, nil, nil,
				false, false, false, true, nil, nil, false, DescribeStacksErrorOptions{StaticOnly: true})
			if tc.wantErr == nil {
				require.NoError(t, err)
				require.Contains(t, result, "dev")
			} else {
				require.ErrorIs(t, err, tc.wantErr)
				if errors.Is(tc.wantErr, errUtils.ErrInvalidImport) {
					require.Contains(t, err.Error(), "cannot inspect templated import")
				}
			}
			require.NoFileExists(t, marker, "static inspection must not execute locals or import-path templates")
			actual, readErr := os.ReadFile(file)
			require.NoError(t, readErr)
			require.Equal(t, source, string(actual), "static inspection must not rewrite the manifest")
			require.Zero(t, requests.Load(), "static inspection must not fetch imports")
		})
	}
}

func TestDescribeStacksStaticEmptyStacks(t *testing.T) {
	cases := []struct {
		name         string
		source       string
		filter       string
		includeEmpty bool
		wantStack    bool
		wantApp      bool
		wantErr      bool
	}{
		{name: "include empty logical stack", source: "vars: {stage: dev}\n", includeEmpty: true, wantStack: true},
		{name: "exclude empty logical stack", source: "vars: {stage: dev}\n"},
		{name: "filter unrelated logical stack", source: "vars: {stage: dev}\n", filter: "prod", includeEmpty: true},
		{
			name:         "unrequested component type leaves empty stack",
			source:       "vars: {stage: dev}\ncomponents:\n  helmfile:\n    app: {}\n",
			includeEmpty: true,
			wantStack:    true,
		},
		{
			name:         "requested components are retained",
			source:       "vars: {stage: dev}\ncomponents:\n  terraform:\n    app: {}\n",
			includeEmpty: true,
			wantStack:    true,
			wantApp:      true,
		},
		{name: "unresolved empty stack name fails", source: "vars: {other: value}\n", includeEmpty: true, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			file := filepath.Join(root, "physical.yaml")
			require.NoError(t, os.WriteFile(file, []byte(tc.source), 0o600))
			config := &schema.AtmosConfiguration{StacksBaseAbsolutePath: root, StackConfigFilesAbsolutePaths: []string{file}}
			config.Stacks.NameTemplate = "{{ .vars.stage }}"
			result, err := ExecuteDescribeStacksWithOptions(config, tc.filter, nil, []string{"terraform"}, nil,
				false, false, false, tc.includeEmpty, nil, nil, false, DescribeStacksErrorOptions{StaticOnly: true})
			if tc.wantErr {
				require.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
				require.Contains(t, err.Error(), "cannot determine stack name")
				return
			}
			require.NoError(t, err)
			if !tc.wantStack {
				require.Empty(t, result)
				return
			}
			require.Len(t, result, 1, "the physical filename must not become a second stack")
			stack, ok := result["dev"].(map[string]any)
			require.True(t, ok, "the merged vars must determine the logical name")
			components, ok := stack["components"].(map[string]any)
			require.True(t, ok)
			if tc.wantApp {
				require.Len(t, components, 1)
				terraform, ok := components["terraform"].(map[string]any)
				require.True(t, ok)
				require.Contains(t, terraform, "app")
			} else {
				require.Empty(t, components)
			}
		})
	}
}
