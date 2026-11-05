package doctor

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalSchemaUnreadableAndInvalidCopies(t *testing.T) {
	for _, tc := range []struct{ name, content, reason string }{
		{"missing", "", "Local schema could not be read"},
		{"invalid JSON", "{broken", "Local schema is not valid JSON"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", "schemas:\n  atmos:\n    manifest: schema.json\n")
			if tc.content != "" {
				projectFile(t, root, "schema.json", tc.content)
			}
			report := ecRun(t, &Options{Root: root, Only: []string{ruleLocalAtmosSchema}, Offline: true, Fix: true})
			findings := ecFindings(&report, ruleLocalAtmosSchema)
			require.Len(t, findings, 1)
			require.Contains(t, findings[0].Reason, tc.reason)
			require.Contains(t, findings[0].Remediation, "Remove the local override")
			require.False(t, findings[0].Fixable)
			require.Empty(t, report.ChangedFiles)
		})
	}
}

func TestLocalSchemaLookupPrecedence(t *testing.T) {
	root := newProject(t)
	base := filepath.Join(root, "base")
	baseSchema := projectFile(t, root, "base/schema.json", `{}`)
	e := testEngine(t)
	require.NoError(t, e.configure(&Options{Root: root}))
	p := &project{root: root, base: base}
	require.Equal(t, baseSchema, e.localSchemaPath(p, "schema.json"))
	rootSchema := projectFile(t, root, "schema.json", `{}`)
	require.Equal(t, rootSchema, e.localSchemaPath(p, "schema.json"))
	require.Equal(t, baseSchema, e.localSchemaPath(p, baseSchema))
	missing := filepath.Join(root, "missing.json")
	require.Equal(t, missing, e.localSchemaPath(p, missing), "absolute paths must not fall back to base_path")
}

func TestLocalCustomSchemaPreservedForBothOverrideSources(t *testing.T) {
	for _, source := range []string{"manifest", "config", "environment"} {
		t.Run(source, func(t *testing.T) {
			root := newProject(t)
			t.Setenv("ATMOS_SCHEMAS_ATMOS_MANIFEST", "")
			custom := projectFile(t, root, "custom.json", `{"$id":"https://example.org/constraints","type":"object","required":["owner"]}`)
			config := "{}\n"
			switch source {
			case "manifest":
				config = "schemas:\n  atmos:\n    manifest: custom.json\n"
			case "config":
				config = "schemas:\n  config:\n    schema: custom.json\n"
			case "environment":
				t.Setenv("ATMOS_SCHEMAS_ATMOS_MANIFEST", custom)
			}
			projectFile(t, root, "atmos.yaml", config)
			report, err := testEngine(t).Run(context.Background(), Options{Root: root, Only: []string{ruleLocalAtmosSchema}, Offline: true, Fix: true})
			require.NoError(t, err)
			findings := ecFindings(&report, ruleLocalAtmosSchema)
			require.Len(t, findings, 1)
			require.Equal(t, customLocalSchemaMessage, findings[0].Message)
			require.Contains(t, findings[0].Remediation, "keep this file")
			require.NotContains(t, findings[0].Remediation, "Remove the local override")
			require.NotContains(t, findings[0].Remediation, "Unset ATMOS")
			require.Empty(t, report.ChangedFiles)
			require.FileExists(t, custom)
		})
	}
}

func TestLocalSchemaSourcesExcludeRemoteAndDynamicValues(t *testing.T) {
	for _, source := range []string{"", "https://example.org/schema.json", configSchema, stackSchema, "{{ .vars.schema }}", "!env SCHEMA"} {
		require.False(t, localSchemaSource(source), source)
	}
	for _, source := range []string{"schema.json", "schemas/manifest.json", filepath.Join(t.TempDir(), "schema.json")} {
		require.True(t, localSchemaSource(source), source)
	}
}
