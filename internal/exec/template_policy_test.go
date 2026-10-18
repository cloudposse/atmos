package exec

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hairyhenderson/gomplate/v3/data"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/policy/guard"
	"github.com/cloudposse/atmos/pkg/schema"
)

func templatePolicy(t *testing.T, kind, rule string, roots []string) {
	t.Helper()
	directory := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", directory)
	manifest := fmt.Sprintf("apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: template}\nspec:\n  policies:\n    guard:\n      on: %s\n      action: deny\n      rule: %q\n", kind, rule)
	if len(roots) > 0 {
		manifest += "      allowed_roots:\n"
		for _, root := range roots {
			manifest += fmt.Sprintf("        - %q\n", root)
		}
	}
	require.NoError(t, os.WriteFile(filepath.Join(directory, "policy.yaml"), []byte(manifest), 0o600))
}

func TestTemplateFunctionAdmission(t *testing.T) {
	templatePolicy(t, "function", "input.engine == 'go-template' && input.name in ['env', 'file.Read', 'atmos.Resolve']", nil)
	t.Setenv("POLICY_TEST_SECRET", "must-not-be-rendered")
	for _, source := range []string{`{{ env "POLICY_TEST_SECRET" }}`, `{{ file.Read "does-not-exist" }}`, `{{ $f := file }}{{ $f.Read "does-not-exist" }}`, `{{ atmos.Resolve "!env POLICY_TEST_SECRET" }}`} {
		rendered, err := ProcessTmpl(&schema.AtmosConfiguration{}, "test", source, nil, false)
		require.Error(t, err, source)
		require.Contains(t, err.Error(), "policy")
		require.NotContains(t, rendered, "must-not-be-rendered")
	}
	rendered, err := ProcessTmpl(&schema.AtmosConfiguration{}, "test", `{{ if false }}{{ env "POLICY_TEST_SECRET" }}{{ end }}{{ add 1 2 }}`, nil, false)
	require.NoError(t, err)
	require.Equal(t, "3", rendered, "unreached functions must not be denied")
}

func TestTemplateFileConfinement(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "allowed")
	require.NoError(t, os.Mkdir(allowed, 0o700))
	inside := filepath.Join(allowed, "public.txt")
	outside := filepath.Join(root, "secret.txt")
	require.NoError(t, os.WriteFile(inside, []byte("public"), 0o600))
	require.NoError(t, os.WriteFile(outside, []byte("secret"), 0o600))
	templatePolicy(t, "source", "input.scheme == 'file' && !input.within_allowed_roots", []string{allowed})
	for _, tc := range []struct {
		file   string
		denied bool
	}{{inside, false}, {outside, true}} {
		rendered, err := ProcessTmpl(&schema.AtmosConfiguration{}, "test", fmt.Sprintf("{{ file.Read %q }}", tc.file), nil, false)
		if tc.denied {
			require.Error(t, err)
			require.Empty(t, rendered)
		} else {
			require.NoError(t, err)
			require.Equal(t, "public", rendered)
		}
	}
}

func TestTemplateDatasourceRedirectConfinement(t *testing.T) {
	var reached atomic.Int32
	forbidden := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached.Add(1); _, _ = w.Write([]byte("secret")) }))
	defer forbidden.Close()
	// Different host spelling distinguishes allowed first hop from the forbidden redirect.
	forbiddenURL := strings.Replace(forbidden.URL, "127.0.0.1", "localhost", 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, forbiddenURL, http.StatusFound) }))
	defer server.Close()
	templatePolicy(t, "source", "input.host == 'localhost'", nil)
	_, err := ProcessTmpl(&schema.AtmosConfiguration{}, "test", fmt.Sprintf("{{ include %q }}", server.URL), nil, false)
	require.Error(t, err)
	require.Zero(t, reached.Load())
}

func TestTemplateSourceNamespacePaths(t *testing.T) {
	templatePolicy(t, "source", "true", nil)
	for _, source := range []string{
		`{{ ds "https://example.invalid/data.json" }}`,
		`{{ net.LookupIP "example.invalid" }}`,
		`{{ $n := net }}{{ $n.LookupIP "example.invalid" }}`,
		`{{ net.NetFuncs.LookupIP "example.invalid" }}`,
		`{{ aws }}`, `{{ gcp }}`,
	} {
		_, err := ProcessTmpl(&schema.AtmosConfiguration{}, "test", source, nil, false)
		require.Error(t, err, source)
	}
	rendered, err := ProcessTmpl(&schema.AtmosConfiguration{}, "test", `{{ net.ParseIP "192.0.2.1" }}`, nil, false)
	require.NoError(t, err)
	require.Equal(t, "192.0.2.1", rendered)
}

func TestTemplateEJSONFailsBeforeImplicitKeyRead(t *testing.T) {
	templatePolicy(t, "source", "false", nil)
	for _, source := range []string{
		`{{ json "{\"_public_key\":\"invalid-key\"}" }}`,
		`{{ data.JSON "{\"_public_key\":\"invalid-key\"}" }}`,
	} {
		_, err := ProcessTmpl(&schema.AtmosConfiguration{}, "test", source, nil, false)
		require.ErrorContains(t, err, "EJSON key reads have no guarded collector")
	}
}

func TestTemplateFunctionProvenance(t *testing.T) {
	guard.ResetOrigins()
	t.Cleanup(guard.ResetOrigins)
	file := filepath.Join(t.TempDir(), "template.yaml")
	require.NoError(t, os.WriteFile(file, []byte("template"), 0o600))
	templatePolicy(t, "function", "input.engine == 'go-template' && input.name == 'env' && input.provenance != 'local'", nil)
	t.Setenv("POLICY_ORIGIN", "local")
	source := `{{ env "POLICY_ORIGIN" }}`
	rendered, err := ProcessTmpl(&schema.AtmosConfiguration{}, file, source, nil, false)
	require.NoError(t, err)
	require.Equal(t, "local", rendered)
	guard.RegisterRemote(file, "https://example.com/template.yaml")
	_, err = ProcessTmpl(&schema.AtmosConfiguration{}, file, source, nil, false)
	require.Error(t, err)
}

func TestExplainFlagIsNotForwarded(t *testing.T) {
	for _, flag := range []string{"--explain", "--explain=true", "--explain=false"} {
		info, err := processArgsAndFlags("terraform", []string{"plan", "vpc", flag, "-lock=false"})
		require.NoError(t, err)
		require.Equal(t, "vpc", info.ComponentFromArg)
		require.Equal(t, []string{"-lock=false"}, info.AdditionalArgsAndFlags)
	}
}

func TestStackSourcePolicyBeforeCachedRead(t *testing.T) {
	ClearFileContentCache()
	t.Cleanup(ClearFileContentCache)
	file := filepath.Join(t.TempDir(), "stack.yaml")
	require.NoError(t, os.WriteFile(file, []byte("secret"), 0o600))
	templatePolicy(t, "source", "false", nil)
	_, err := GetFileContent(file)
	require.NoError(t, err)
	templatePolicy(t, "source", "true", nil)
	_, err = GetFileContent(file)
	require.Error(t, err)
	_, err = GetFileContentWithoutCache(file)
	require.Error(t, err)
}

func TestPolicyCoversAllTemplateRenderers(t *testing.T) {
	templatePolicy(t, "function", "input.engine == 'go-template' && input.name == 'file.Read'", nil)
	config := &schema.AtmosConfiguration{}
	config.Templates.Settings.Enabled = true
	config.Templates.Settings.Gomplate.Enabled = true
	source := `{{ file.Read "must-not-be-read" }}`
	_, err := ProcessTmplWithDatasources(config, &schema.ConfigAndStacksInfo{}, schema.Settings{}, "test", source, map[string]any{}, false)
	require.ErrorContains(t, err, "policy")
	_, err = ProcessTmplWithDatasourcesGomplate(config, "test", source, map[string]any{}, false)
	require.ErrorContains(t, err, "policy")
}

func TestOptionalStackImportsDoNotSuppressPolicies(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "stack.yaml")
	require.NoError(t, os.WriteFile(file, []byte("vars: {value: secret}\n"), 0o600))
	templatePolicy(t, "source", "true", nil)
	_, err := ProcessYAMLConfigFile(&schema.AtmosConfiguration{}, root, file, map[string]map[string]any{}, nil, true, true, false, true, nil, nil, nil, nil, "")
	require.ErrorContains(t, err, "policy")
}

// TestTemplateGuardedJSONKeepsIntegers verifies source-guarded JSON decoding matches native
// gomplate behavior: whole numbers stay integers instead of rendering as float64 (`1e+06`).
func TestTemplateGuardedJSONKeepsIntegers(t *testing.T) {
	templatePolicy(t, "source", "false", nil)

	t.Run("guardedTemplateJSON decodes whole numbers as int", func(t *testing.T) {
		result, err := guardedTemplateJSON(`{"n":1000000,"f":1.5}`)
		require.NoError(t, err)
		require.Equal(t, 1000000, result["n"])
		require.Equal(t, 1.5, result["f"])
	})

	t.Run("parsePolicyJSONDatasource decodes whole numbers as int", func(t *testing.T) {
		result, err := parsePolicyJSONDatasource([]byte(`{"n":1000000}`))
		require.NoError(t, err)
		object, ok := result.(map[string]any)
		require.True(t, ok)
		require.Equal(t, 1000000, object["n"])
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"n":1000000}`))
	}))
	defer server.Close()

	for name, source := range map[string]string{
		"json":       `{{ (json "{\"n\":1000000}").n }}`,
		"data.JSON":  `{{ (data.JSON "{\"n\":1000000}").n }}`,
		"datasource": fmt.Sprintf(`{{ (ds %q).n }}`, server.URL),
	} {
		t.Run("renders "+name, func(t *testing.T) {
			rendered, err := ProcessTmpl(&schema.AtmosConfiguration{}, "test", source, nil, false)
			require.NoError(t, err)
			require.Equal(t, "1000000", rendered)
		})
	}

	t.Run("EJSON rejection is unchanged", func(t *testing.T) {
		_, err := guardedTemplateJSON(`{"_public_key":"x"}`)
		require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
		_, err = parsePolicyJSONDatasource([]byte(`{"_public_key":"x"}`))
		require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	})
}

// TestTemplateDatasourceReachableIsBooleanProbe verifies datasourceReachable reports ordinary
// failures as false (so `else` branches run) while still failing closed on policy outcomes.
func TestTemplateDatasourceReachableIsBooleanProbe(t *testing.T) {
	const probe = `{{ if datasourceReachable %q }}reachable{{ else }}unreachable{{ end }}`

	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
	defer failing.Close()
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer healthy.Close()
	closed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closedURL := closed.URL
	closed.Close()

	t.Run("ordinary failures return false without aborting the render", func(t *testing.T) {
		templatePolicy(t, "source", "false", nil)
		for name, alias := range map[string]string{
			"http 500 status":    failing.URL,
			"connection refused": closedURL,
			"unknown datasource": "no-such-alias",
			"unsupported scheme": "ftp://example.invalid/data",
		} {
			rendered, err := ProcessTmpl(&schema.AtmosConfiguration{}, "test", fmt.Sprintf(probe, alias), nil, false)
			require.NoError(t, err, name)
			require.Equal(t, "unreachable", rendered, name)
		}
	})

	t.Run("reachable source returns true", func(t *testing.T) {
		templatePolicy(t, "source", "false", nil)
		rendered, err := ProcessTmpl(&schema.AtmosConfiguration{}, "test", fmt.Sprintf(probe, healthy.URL), nil, false)
		require.NoError(t, err)
		require.Equal(t, "reachable", rendered)
	})

	t.Run("denied source fails closed", func(t *testing.T) {
		templatePolicy(t, "source", "true", nil)
		for _, alias := range []string{healthy.URL, failing.URL} {
			rendered, err := ProcessTmpl(&schema.AtmosConfiguration{}, "test", fmt.Sprintf(probe, alias), nil, false)
			require.ErrorContains(t, err, "policy")
			require.NotEqual(t, "unreachable", rendered)
		}
	})

	t.Run("wrapper propagates only genuine policy outcomes", func(t *testing.T) {
		templatePolicy(t, "source", "true", nil)
		probeFn, ok := policySourceFunction("datasourceReachable", nil, "", &data.Data{}).(func(string, ...string) (bool, error))
		require.True(t, ok)
		reachable, err := probeFn(healthy.URL)
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
		require.False(t, reachable)

		templatePolicy(t, "source", "false", nil)
		reachable, err = probeFn(failing.URL)
		require.NoError(t, err)
		require.False(t, reachable)
	})
}
