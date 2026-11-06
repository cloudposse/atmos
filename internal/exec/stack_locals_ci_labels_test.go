package exec

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci"
	githubprovider "github.com/cloudposse/atmos/pkg/ci/providers/github"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Collection-argument tags inside locals must load under every edition. The explicit-tag path
// (2026-10-20 and unpinned) previously rejected a !ci.labels sequence after a YAML template pass
// restored its argument as a quoted scalar.
func TestProcessYAMLConfigFileCollectionTagsInLocals(t *testing.T) {
	// Without a CI review, !ci.labels membership is false and list mode is empty.
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("CI", "")

	tests := []struct {
		name     string
		manifest string
		key      string
		want     any
	}{
		{"ci.labels block sequence", "vars:\n  stage: dev\nlocals:\n  value: !ci.labels\n    - preview\n", "value", false},
		{"ci.labels flow sequence", "vars:\n  stage: dev\nlocals:\n  value: !ci.labels [preview, deploy]\n", "value", false},
		{"ci.labels bare tag", "vars:\n  stage: dev\nlocals:\n  value: !ci.labels\n", "value", []any{}},
		{"ci.labels section-specific locals", "vars:\n  stage: dev\nterraform:\n  locals:\n    value: !ci.labels [preview]\n", "value", false},
		{"ci.labels nested locals value", "vars:\n  stage: dev\nlocals:\n  ci:\n    value: !ci.labels [preview]\n", "ci", map[string]any{"value": false}},
		{"literal sequence", "vars:\n  stage: dev\nlocals:\n  value: !literal [a, b]\n", "value", []any{"a", "b"}},
		{"literal mapping", "vars:\n  stage: dev\nlocals:\n  value: !literal {a: 1}\n", "value", map[string]any{"a": 1}},
	}
	for _, edition := range []string{"", "2026-10-20", "2026-10-19"} {
		for _, tt := range tests {
			t.Run(edition+"/"+tt.name, func(t *testing.T) {
				ClearLocalsExtractionCache()
				ClearFileContentCache()
				dir := writeStackFixture(t, map[string]string{"dev.yaml": tt.manifest})
				atmosConfig := &schema.AtmosConfiguration{Edition: edition}
				atmosConfig.Templates.Settings.Enabled = true

				result, err := extractLocalsFromRawYAML(atmosConfig, tt.manifest, filepath.Join(dir, "dev.yaml"))
				require.NoError(t, err)
				assert.Equal(t, tt.want, result.locals[tt.key])

				_, err = ProcessYAMLConfigFileWithOptions(atmosConfig, dir, filepath.Join(dir, "dev.yaml"), StackLoadOptions{})
				require.NoError(t, err)
			})
		}
	}
}

// With a review that carries the requested label, the locals path must keep the argument intact and
// answer true. The CI-disabled cases above all expect false, so they cannot detect a lost argument.
func TestExtractLocalsCILabelsMatchesProviderLabels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/issues/42/labels", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[{"name":"preview"}]`)
	}))
	t.Cleanup(server.Close)
	eventPath := filepath.Join(t.TempDir(), "event.json")
	require.NoError(t, os.WriteFile(eventPath, []byte(`{"pull_request":{"number":42}}`), 0o600))
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_EVENT_PATH", eventPath)
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	t.Setenv("ATMOS_CI_GITHUB_TOKEN", "labels-test-token")
	t.Setenv("ATMOS_CI_GITHUB_API_URL", server.URL+"/")
	t.Cleanup(ci.SwapRegistryForTest())
	ci.Register(githubprovider.NewProvider())
	ci.ResetLabelsCache()
	t.Cleanup(ci.ResetLabelsCache)

	tests := []struct {
		name     string
		manifest string
		want     any
	}{
		{"matching label", "vars:\n  stage: dev\nlocals:\n  value: !ci.labels [preview]\n", true},
		{"other label", "vars:\n  stage: dev\nlocals:\n  value: !ci.labels [deploy]\n", false},
		{"list mode", "vars:\n  stage: dev\nlocals:\n  value: !ci.labels\n", []any{"preview"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ClearLocalsExtractionCache()
			ClearFileContentCache()
			dir := writeStackFixture(t, map[string]string{"dev.yaml": tt.manifest})
			atmosConfig := &schema.AtmosConfiguration{}
			atmosConfig.Templates.Settings.Enabled = true

			result, err := extractLocalsFromRawYAML(atmosConfig, tt.manifest, filepath.Join(dir, "dev.yaml"))
			require.NoError(t, err)
			assert.Equal(t, tt.want, result.locals["value"])
		})
	}
}

// Negative path: a quoted scalar argument is still rejected under every edition. Only the
// restored sequence form is accepted, so the fix does not relax !ci.labels validation.
func TestProcessYAMLConfigFileCILabelsQuotedScalarStillRejected(t *testing.T) {
	for _, edition := range []string{"", "2026-10-19"} {
		t.Run(edition, func(t *testing.T) {
			ClearLocalsExtractionCache()
			ClearFileContentCache()
			manifest := "vars:\n  stage: dev\nlocals:\n  value: !ci.labels \"[preview]\"\n"
			dir := writeStackFixture(t, map[string]string{"dev.yaml": manifest})

			_, err := ProcessYAMLConfigFileWithOptions(&schema.AtmosConfiguration{Edition: edition}, dir, filepath.Join(dir, "dev.yaml"), StackLoadOptions{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "got the scalar")
		})
	}
}
