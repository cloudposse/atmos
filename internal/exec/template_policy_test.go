package exec

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/policy/guard"
	"github.com/cloudposse/atmos/pkg/schema"
	data "github.com/cloudposse/atmos/pkg/template/gomplate"
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

// templatePolicyInvalid points ATMOS_POLICY_DIR at an empty directory so loading the bundle fails.
func templatePolicyInvalid(t *testing.T) {
	t.Helper()
	t.Setenv("ATMOS_POLICY_DIR", t.TempDir())
}

// templateDatasourceURL builds a file:// datasource URL for a local path on every platform.
func templateDatasourceURL(path string) string {
	u := url.URL{Scheme: "file", Path: "/" + strings.TrimPrefix(filepath.ToSlash(path), "/")}
	return u.String()
}

// newTemplateData builds gomplate datasources from alias=URL definitions and optional header definitions.
func newTemplateData(t *testing.T, sources, headers []string) *data.Data {
	t.Helper()
	d, err := data.NewData(sources, headers)
	require.NoError(t, err)
	return d
}

// TestPolicyDataFuncsAreAdmittedPerFunction verifies every guarded data.* function runs when allowed and
// fails closed with the policy denial when a function rule names it.
func TestPolicyDataFuncsAreAdmittedPerFunction(t *testing.T) {
	const jsonLiteral = `{\"a\":1}`
	tests := []struct {
		name     string
		template string
		expected string
	}{
		{"data.JSON", `{{ (data.JSON "` + jsonLiteral + `").a }}`, "1"},
		{"data.JSONArray", `{{ index (data.JSONArray "[1,2]") 1 }}`, "2"},
		{"data.YAML", `{{ (data.YAML "a: b").a }}`, "b"},
		{"data.YAMLArray", `{{ index (data.YAMLArray "- x\n- y") 1 }}`, "y"},
		{"data.TOML", `{{ (data.TOML "a = \"b\"").a }}`, "b"},
		{"data.CSV", `{{ index (index (data.CSV "a,b\n1,2") 1) 1 }}`, "2"},
		{"data.CSVByRow", `{{ (index (data.CSVByRow "a,b\n1,2") 0).b }}`, "2"},
		{"data.CSVByColumn", `{{ index (data.CSVByColumn "a,b\n1,2").b 0 }}`, "2"},
		{"data.ToCSV", `{{ data.ToCSV (data.CSV "a,b\n1,2") }}`, "a,b\r\n1,2\r\n"},
		{"data.ToJSON", `{{ data.ToJSON (data.JSON "` + jsonLiteral + `") }}`, `{"a":1}`},
		{"data.ToJSONPretty", `{{ data.ToJSONPretty "  " (data.JSON "` + jsonLiteral + `") }}`, "{\n  \"a\": 1\n}"},
		{"data.ToYAML", `{{ data.ToYAML (data.JSON "` + jsonLiteral + `") }}`, "a: 1\n"},
		{"data.ToTOML", `{{ data.ToTOML (data.JSON "` + jsonLiteral + `") }}`, "a = 1\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// A rule that names a different function must leave the call untouched.
			templatePolicy(t, "function", "input.engine == 'go-template' && input.name == 'data.NoSuchFunction'", nil)
			rendered, err := ProcessTmpl(&schema.AtmosConfiguration{}, "test", tc.template, nil, false)
			require.NoError(t, err)
			require.Equal(t, tc.expected, rendered)

			// A rule that names exactly this function must deny it and render nothing.
			templatePolicy(t, "function", fmt.Sprintf("input.engine == 'go-template' && input.name == %q", tc.name), nil)
			rendered, err = ProcessTmpl(&schema.AtmosConfiguration{}, "test", tc.template, nil, false)
			require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
			require.NotEqual(t, tc.expected, rendered)
		})
	}
}

// TestPolicyDataFuncsPropagateNativeErrors verifies admitted data.* calls still surface the native decode error.
func TestPolicyDataFuncsPropagateNativeErrors(t *testing.T) {
	templatePolicy(t, "function", "input.name == 'data.NoSuchFunction'", nil)
	for name, source := range map[string]string{
		"data.JSONArray": `{{ data.JSONArray "{" }}`,
		"data.YAML":      `{{ data.YAML "a: [" }}`,
		"data.YAMLArray": `{{ data.YAMLArray "a: [" }}`,
		"data.TOML":      `{{ data.TOML "a = " }}`,
		"data.CSV":       `{{ data.CSV "a,\"b" }}`,
	} {
		t.Run(name, func(t *testing.T) {
			rendered, err := ProcessTmpl(&schema.AtmosConfiguration{}, "test", source, nil, false)
			require.Error(t, err)
			require.NotErrorIs(t, err, errUtils.ErrPolicyDenied)
			require.Empty(t, rendered)
		})
	}
}

func TestGuardedTemplateJSON(t *testing.T) {
	t.Run("rule lookup failure is returned", func(t *testing.T) {
		templatePolicyInvalid(t)
		result, err := guardedTemplateJSON(`{"a":1}`)
		require.ErrorIs(t, err, errUtils.ErrNoPoliciesConfigured)
		require.Nil(t, result)
	})

	t.Run("without source rules the native decoder is used", func(t *testing.T) {
		templatePolicy(t, "function", "false", nil)
		result, err := guardedTemplateJSON(`{"a":"b"}`)
		require.NoError(t, err)
		require.Equal(t, map[string]any{"a": "b"}, result)

		// The native decoder reads EJSON key files, which is why only the guarded path rejects them.
		_, err = guardedTemplateJSON(`{`)
		require.Error(t, err)
	})

	t.Run("with source rules malformed input fails", func(t *testing.T) {
		templatePolicy(t, "source", "false", nil)
		result, err := guardedTemplateJSON(`{"a": [`)
		require.Error(t, err)
		require.Nil(t, result)
	})

	t.Run("with source rules non-string input is converted", func(t *testing.T) {
		templatePolicy(t, "source", "false", nil)
		result, err := guardedTemplateJSON([]byte(`{"a":"b"}`))
		require.NoError(t, err)
		require.Equal(t, map[string]any{"a": "b"}, result)
	})

	t.Run("with source rules EJSON documents are rejected", func(t *testing.T) {
		templatePolicy(t, "source", "false", nil)
		result, err := guardedTemplateJSON(`{"_public_key":"abc","a":"b"}`)
		require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
		require.Nil(t, result)
	})
}

func TestParsePolicyDatasourceFormats(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		path      string
		mediaType string
		raw       bool
		expected  any
	}{
		{"raw skips decoding", `{"a":1}`, "x.json", "application/json", true, `{"a":1}`},
		{"json by media type", `{"a":1}`, "", "application/json; charset=utf-8", false, map[string]any{"a": 1}},
		{"json by extension", `{"a":1}`, "dir/x.json", "", false, map[string]any{"a": 1}},
		{"ejson extension without key", `{"a":1}`, "x.ejson", "", false, map[string]any{"a": 1}},
		{"json array", `[1,"b"]`, "x.json", "", false, []any{1, "b"}},
		{"yaml by media type", "a: b\n", "", "application/x-yaml", false, map[string]any{"a": "b"}},
		{"yaml by yaml extension", "a: b\n", "x.yaml", "", false, map[string]any{"a": "b"}},
		{"yaml by yml extension", "- a\n- b\n", "x.YML", "", false, []any{"a", "b"}},
		{"csv by media type", "a,b\n1,2\n", "", "text/csv", false, [][]string{{"a", "b"}, {"1", "2"}}},
		{"csv by extension", "a;b\n1;2\n", "x.csv", "", false, [][]string{{"a;b"}, {"1;2"}}},
		{"toml by media type", "a = \"b\"\n", "", "application/toml", false, map[string]any{"a": "b"}},
		{"toml by extension", "a = 1\n", "x.toml", "", false, map[string]any{"a": int64(1)}},
		{"unknown format stays text", "a: b\n", "x.txt", "text/plain", false, "a: b\n"},
		{"no hints stays text", `{"a":1}`, "", "", false, `{"a":1}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parsePolicyDatasource([]byte(tc.content), tc.path, tc.mediaType, tc.raw)
			require.NoError(t, err)
			require.Equal(t, tc.expected, result)
		})
	}

	failures := []struct {
		name    string
		content string
		path    string
		media   string
	}{
		{"malformed json", `{"a": [`, "x.json", ""},
		{"malformed yaml", "a: [", "x.yaml", ""},
		{"malformed csv", "a,\"b\n", "x.csv", ""},
		{"malformed toml", "a = ", "x.toml", ""},
	}
	for _, tc := range failures {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parsePolicyDatasource([]byte(tc.content), tc.path, tc.media, false)
			require.Error(t, err)
			require.Nil(t, result)
		})
	}

	t.Run("EJSON document is rejected", func(t *testing.T) {
		result, err := parsePolicyDatasource([]byte(`{"_public_key":"abc"}`), "secrets.ejson", "", false)
		require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
		require.Nil(t, result)
	})
}

func TestPolicyDatasourceTarget(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")

	t.Run("invalid alias", func(t *testing.T) {
		_, _, err := policyDatasourceTarget(newTemplateData(t, nil, nil), "%zz", nil)
		require.ErrorIs(t, err, errUtils.ErrPolicyInputInvalid)
	})

	t.Run("source without URL is unavailable", func(t *testing.T) {
		d := newTemplateData(t, nil, nil)
		d.Sources["broken"] = &data.Source{Alias: "broken"}
		_, _, err := policyDatasourceTarget(d, "broken", nil)
		require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	})

	t.Run("more than one subpath is unavailable", func(t *testing.T) {
		_, _, err := policyDatasourceTarget(newTemplateData(t, nil, nil), "https://example.invalid/", []string{"a", "b"})
		require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	})

	t.Run("invalid subpath", func(t *testing.T) {
		_, _, err := policyDatasourceTarget(newTemplateData(t, nil, nil), "https://example.invalid/", []string{"%zz"})
		require.ErrorIs(t, err, errUtils.ErrPolicyInputInvalid)
	})

	t.Run("alias resolves to its definition and headers", func(t *testing.T) {
		d := newTemplateData(t, []string{"api=https://example.invalid/base/"}, []string{"api=Authorization: Bearer token"})
		target, headers, err := policyDatasourceTarget(d, "api", nil)
		require.NoError(t, err)
		require.Equal(t, "https://example.invalid/base/", target.String())
		require.Equal(t, []string{"Bearer token"}, headers["Authorization"])
		// The returned target is a copy so later subpaths cannot mutate the registered source.
		target.Path = "/changed"
		require.Equal(t, "/base/", d.Sources["api"].URL.Path)
	})

	t.Run("http subpath resolves against the source", func(t *testing.T) {
		d := newTemplateData(t, []string{"api=https://example.invalid/base/"}, nil)
		target, _, err := policyDatasourceTarget(d, "api", []string{"item.json"})
		require.NoError(t, err)
		require.Equal(t, "https://example.invalid/base/item.json", target.String())
	})

	t.Run("file subpath is joined and cleaned", func(t *testing.T) {
		d := newTemplateData(t, []string{"files=" + templateDatasourceURL(dir) + "/"}, nil)
		target, _, err := policyDatasourceTarget(d, "files", []string{"sub/../x.json"})
		require.NoError(t, err)
		require.Equal(t, "file", target.Scheme)
		require.Equal(t, "/x.json", target.Path[strings.LastIndex(target.Path, "/"):])
		require.NotContains(t, target.Path, "..")
	})
}

func TestPolicyDatasourceWithoutSourceRulesUsesNativeReaders(t *testing.T) {
	templatePolicy(t, "function", "false", nil)
	// An HTTP source keeps this portable: gomplate cannot read file:///C:/... URLs on Windows.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"a":"b"}`))
	}))
	t.Cleanup(server.Close)
	d := newTemplateData(t, []string{"values=" + server.URL + "/values.json"}, nil)

	parsed, err := policyDatasource(d, "test", "values", false)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"a": "b"}, parsed)

	raw, err := policyDatasource(d, "test", "values", true)
	require.NoError(t, err)
	require.Equal(t, `{"a":"b"}`, raw)
}

func TestPolicyDatasourceRuleLookupFailureIsReturned(t *testing.T) {
	templatePolicyInvalid(t)
	d := newTemplateData(t, nil, nil)
	result, err := policyDatasource(d, "test", "https://example.invalid/", false)
	require.ErrorIs(t, err, errUtils.ErrNoPoliciesConfigured)
	require.Nil(t, result)
}

func TestPolicyDatasourceTargetErrorsFailBeforeIO(t *testing.T) {
	templatePolicy(t, "source", "false", nil)
	d := newTemplateData(t, nil, nil)
	d.Sources["broken"] = &data.Source{Alias: "broken"}

	_, err := policyDatasource(d, "test", "%zz", false)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputInvalid)
	_, err = policyDatasource(d, "test", "broken", false)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	_, err = policyDatasource(d, "test", "https://example.invalid/", false, "a", "b")
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
}

func TestPolicyDatasourceUnguardedSchemeIsUnavailable(t *testing.T) {
	templatePolicy(t, "source", "false", nil)
	d := newTemplateData(t, nil, nil)
	result, err := policyDatasource(d, "test", "ftp://example.invalid/data.json", false)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	require.Nil(t, result)
}

func TestPolicyDatasourceLocalFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("gomplate reads file datasources through filepath.FromSlash and cannot open file:///C:/... URLs on Windows")
	}
	root := t.TempDir()
	allowed := filepath.Join(root, "allowed")
	require.NoError(t, os.Mkdir(allowed, 0o700))
	write := func(dir, name, content string) string {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
		return path
	}
	jsonFile := write(allowed, "config.json", `{"n":2000000,"name":"x"}`)
	yamlFile := write(allowed, "config.yaml", "list:\n  - a\n  - b\n")
	csvFile := write(allowed, "config.csv", "k,v\n1,2\n")
	tomlFile := write(allowed, "config.toml", "k = \"v\"\n")
	textFile := write(allowed, "config.txt", "plain: text\n")
	ejsonFile := write(allowed, "secrets.ejson", `{"_public_key":"abc"}`)
	badJSON := write(allowed, "bad.json", `{"n": [`)
	outsideFile := write(root, "secret.json", `{"secret":true}`)
	templatePolicy(t, "source", "input.scheme == 'file' && !input.within_allowed_roots", []string{allowed})

	tests := []struct {
		name     string
		file     string
		raw      bool
		expected any
	}{
		{"json keeps integers", jsonFile, false, map[string]any{"n": 2000000, "name": "x"}},
		{"yaml", yamlFile, false, map[string]any{"list": []any{"a", "b"}}},
		{"csv", csvFile, false, [][]string{{"k", "v"}, {"1", "2"}}},
		{"toml", tomlFile, false, map[string]any{"k": "v"}},
		{"unknown extension is text", textFile, false, "plain: text\n"},
		{"include returns raw content", jsonFile, true, `{"n":2000000,"name":"x"}`},
	}
	for _, tc := range tests {
		t.Run("allowed "+tc.name, func(t *testing.T) {
			d := newTemplateData(t, []string{"cfg=" + templateDatasourceURL(tc.file)}, nil)
			result, err := policyDatasource(d, "test", "cfg", tc.raw)
			require.NoError(t, err)
			require.Equal(t, tc.expected, result)
		})
	}

	t.Run("subpath beneath an allowed directory", func(t *testing.T) {
		d := newTemplateData(t, []string{"cfg=" + templateDatasourceURL(allowed) + "/"}, nil)
		result, err := policyDatasource(d, "test", "cfg", false, "config.toml")
		require.NoError(t, err)
		require.Equal(t, map[string]any{"k": "v"}, result)
	})

	t.Run("subpath escaping the allowed directory is denied", func(t *testing.T) {
		d := newTemplateData(t, []string{"cfg=" + templateDatasourceURL(allowed) + "/"}, nil)
		result, err := policyDatasource(d, "test", "cfg", false, filepath.Join("..", "secret.json"))
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
		require.Nil(t, result)
	})

	t.Run("file outside allowed roots is denied", func(t *testing.T) {
		d := newTemplateData(t, []string{"cfg=" + templateDatasourceURL(outsideFile)}, nil)
		result, err := policyDatasource(d, "test", "cfg", false)
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
		require.Nil(t, result)
		result, err = policyDatasource(d, "test", "cfg", true)
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
		require.Nil(t, result)
	})

	t.Run("unresolvable path fails closed", func(t *testing.T) {
		d := newTemplateData(t, []string{"cfg=" + templateDatasourceURL(filepath.Join(allowed, "missing.json"))}, nil)
		result, err := policyDatasource(d, "test", "cfg", false)
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
		require.Nil(t, result)
	})

	t.Run("missing file admitted by policy reports the native read error", func(t *testing.T) {
		templatePolicy(t, "source", "false", nil)
		d := newTemplateData(t, []string{"cfg=" + templateDatasourceURL(filepath.Join(allowed, "missing.json"))}, nil)
		result, err := policyDatasource(d, "test", "cfg", false)
		require.Error(t, err)
		require.NotErrorIs(t, err, errUtils.ErrPolicyDenied)
		require.Nil(t, result)
	})

	t.Run("malformed content fails to decode", func(t *testing.T) {
		d := newTemplateData(t, []string{"cfg=" + templateDatasourceURL(badJSON)}, nil)
		result, err := policyDatasource(d, "test", "cfg", false)
		require.Error(t, err)
		require.Nil(t, result)
	})

	t.Run("EJSON file is rejected unless read raw", func(t *testing.T) {
		d := newTemplateData(t, []string{"cfg=" + templateDatasourceURL(ejsonFile)}, nil)
		result, err := policyDatasource(d, "test", "cfg", false)
		require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
		require.Nil(t, result)
		result, err = policyDatasource(d, "test", "cfg", true)
		require.NoError(t, err)
		require.Equal(t, `{"_public_key":"abc"}`, result)
	})
}

func TestPolicyDatasourceEnvironment(t *testing.T) {
	templatePolicy(t, "source", "false", nil)
	t.Setenv("POLICY_DS_ENV", "from-environment")
	d := newTemplateData(t, []string{"envvar=env:POLICY_DS_ENV"}, nil)
	result, err := policyDatasource(d, "test", "envvar", false)
	require.NoError(t, err)
	require.Equal(t, "from-environment", result)
}

// An env datasource reads a variable, not a file, so a rule that denies files outside the allowed roots
// must admit it instead of describing the variable as a path in the working directory.
func TestPolicyDatasourceEnvironmentIsNotAFile(t *testing.T) {
	templatePolicy(t, "source", fileConfinementRule, []string{t.TempDir()})
	t.Setenv("POLICY_DS_ENV", "from-environment")
	d := newTemplateData(t, []string{"envvar=env:POLICY_DS_ENV"}, nil)
	result, err := policyDatasource(d, "test", "envvar", false)
	require.NoError(t, err)
	require.Equal(t, "from-environment", result)
}

func TestPolicyDatasourceHTTP(t *testing.T) {
	var hits atomic.Int32
	var gotAuth atomic.Value
	mux := http.NewServeMux()
	mux.HandleFunc("/data.json", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		gotAuth.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"n":3000000}`))
	})
	mux.HandleFunc("/data.yaml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("a: b\n"))
	})
	mux.HandleFunc("/data.csv", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte("k,v\n1,2\n"))
	})
	mux.HandleFunc("/missing", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	mux.HandleFunc("/ejson", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"_public_key":"abc"}`))
	})
	mux.HandleFunc("/moved", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/data.json", http.StatusFound)
	})
	mux.HandleFunc("/loop", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/loop", http.StatusFound)
	})
	mux.HandleFunc("/exact", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("a"), policyDatasourceMaxBytes))
	})
	mux.HandleFunc("/huge", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("a"), policyDatasourceMaxBytes+1))
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	templatePolicy(t, "source", "input.scheme == 'http' && input.host == 'localhost'", nil)

	t.Run("json response keeps integers and forwards configured headers", func(t *testing.T) {
		d := newTemplateData(t, []string{"api=" + server.URL + "/data.json"}, []string{"api=Authorization: Bearer token"})
		result, err := policyDatasource(d, "test", "api", false)
		require.NoError(t, err)
		require.Equal(t, map[string]any{"n": 3000000}, result)
		require.Equal(t, "Bearer token", gotAuth.Load())
	})

	t.Run("format is chosen from the path extension or content type", func(t *testing.T) {
		d := newTemplateData(t, nil, nil)
		yamlResult, err := policyDatasource(d, "test", server.URL+"/data.yaml", false)
		require.NoError(t, err)
		require.Equal(t, map[string]any{"a": "b"}, yamlResult)
		csvResult, err := policyDatasource(d, "test", server.URL+"/data.csv", false)
		require.NoError(t, err)
		require.Equal(t, [][]string{{"k", "v"}, {"1", "2"}}, csvResult)
	})

	t.Run("include returns the raw body", func(t *testing.T) {
		d := newTemplateData(t, nil, nil)
		result, err := policyDatasource(d, "test", server.URL+"/data.yaml", true)
		require.NoError(t, err)
		require.Equal(t, "a: b\n", result)
	})

	t.Run("subpath resolves against the base URL", func(t *testing.T) {
		d := newTemplateData(t, []string{"api=" + server.URL + "/"}, nil)
		result, err := policyDatasource(d, "test", "api", false, "data.yaml")
		require.NoError(t, err)
		require.Equal(t, map[string]any{"a": "b"}, result)
	})

	t.Run("redirects to an admitted location are followed", func(t *testing.T) {
		d := newTemplateData(t, nil, nil)
		result, err := policyDatasource(d, "test", server.URL+"/moved", false)
		require.NoError(t, err)
		require.Equal(t, map[string]any{"n": 3000000}, result)
	})

	t.Run("redirect loops stop after the limit", func(t *testing.T) {
		d := newTemplateData(t, nil, nil)
		result, err := policyDatasource(d, "test", server.URL+"/loop", false)
		require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
		require.Nil(t, result)
	})

	t.Run("non-200 status is unavailable", func(t *testing.T) {
		d := newTemplateData(t, nil, nil)
		result, err := policyDatasource(d, "test", server.URL+"/missing", false)
		require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
		require.ErrorContains(t, err, "404")
		require.Nil(t, result)
	})

	t.Run("body at the size limit is accepted", func(t *testing.T) {
		d := newTemplateData(t, nil, nil)
		result, err := policyDatasource(d, "test", server.URL+"/exact", true)
		require.NoError(t, err)
		require.Len(t, result, policyDatasourceMaxBytes)
	})

	t.Run("body over the size limit is rejected", func(t *testing.T) {
		d := newTemplateData(t, nil, nil)
		result, err := policyDatasource(d, "test", server.URL+"/huge", true)
		require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
		require.ErrorContains(t, err, "too large")
		require.Nil(t, result)
	})

	t.Run("EJSON response is rejected", func(t *testing.T) {
		d := newTemplateData(t, nil, nil)
		result, err := policyDatasource(d, "test", server.URL+"/ejson", false)
		require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
		require.Nil(t, result)
	})

	t.Run("denied host is never contacted", func(t *testing.T) {
		before := hits.Load()
		localhost := strings.Replace(server.URL, "127.0.0.1", "localhost", 1)
		d := newTemplateData(t, nil, nil)
		result, err := policyDatasource(d, "test", localhost+"/data.json", false)
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
		require.Nil(t, result)
		require.Equal(t, before, hits.Load())

		result, err = policyDatasource(d, "test", server.URL+"/data.json", false)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, before+1, hits.Load())
	})
}

func TestReadPolicyHTTPDatasourceConnectionFailure(t *testing.T) {
	closed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closedURL := closed.URL
	closed.Close()
	templatePolicy(t, "source", "false", nil)
	target, err := url.Parse(closedURL)
	require.NoError(t, err)

	// A data.Data without a context must still make the request.
	result, err := readPolicyHTTPDatasource(&data.Data{}, "test", target, nil, true)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	require.Nil(t, result)
}

func TestReadPolicyHTTPDatasourceRejectsUnbuildableRequest(t *testing.T) {
	target := &url.URL{Scheme: "http", Host: "bad host"}
	result, err := readPolicyHTTPDatasource(&data.Data{}, "test", target, nil, true)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputInvalid)
	require.Nil(t, result)
}

func TestReadPolicyHTTPDatasourceHonorsContextCancellation(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer server.Close()
	templatePolicy(t, "source", "false", nil)
	target, err := url.Parse(server.URL)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := readPolicyHTTPDatasource(&data.Data{Ctx: ctx}, "test", target, nil, true)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	require.Nil(t, result)
	require.Zero(t, hits.Load())
}

// A redirect target that policy denies must surface as the policy denial, never as an unreachable
// source: otherwise datasourceReachable would quietly return false instead of failing closed.
func TestPolicyDatasourceRedirectDenialIsAPolicyOutcome(t *testing.T) {
	var blocked atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/blocked", http.StatusFound) })
	mux.HandleFunc("/blocked", func(w http.ResponseWriter, _ *http.Request) {
		blocked.Add(1)
		_, _ = w.Write([]byte("secret"))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	templatePolicy(t, "source", "input.path == '/blocked'", nil)
	d := newTemplateData(t, nil, nil)

	result, err := policyDatasource(d, "test", server.URL+"/start", false)

	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	require.NotErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	require.Nil(t, result)
	require.Zero(t, blocked.Load(), "the denied redirect target is never contacted")

	probe := fmt.Sprintf(`{{ if datasourceReachable %q }}reachable{{ else }}unreachable{{ end }}`, server.URL+"/start")
	rendered, err := ProcessTmpl(&schema.AtmosConfiguration{}, "test", probe, nil, false)
	require.ErrorContains(t, err, "policy")
	require.NotEqual(t, "unreachable", rendered, "a denied redirect fails closed instead of reading as unreachable")
}
