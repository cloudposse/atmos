package policy

import (
	"bytes"
	"context"
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
	"github.com/cloudposse/atmos/pkg/template/render"
)

// templateDatasourceURL builds a file:// datasource URL for a local path on every platform.
func templateDatasourceURL(path string) string {
	u := url.URL{Scheme: "file", Path: "/" + strings.TrimPrefix(filepath.ToSlash(path), "/")}
	return u.String()
}

func TestGuardedTemplateJSON(t *testing.T) {
	t.Run("rule lookup failure is returned", func(t *testing.T) {
		templatePolicyInvalid(t)
		result, err := guardedTemplateJSON(`{"a":1}`)
		require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
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
		d.Sources["broken"] = &render.Source{Alias: "broken"}
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

	parsed, err := Datasource(d, "test", "values", false)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"a": "b"}, parsed)

	raw, err := Datasource(d, "test", "values", true)
	require.NoError(t, err)
	require.Equal(t, `{"a":"b"}`, raw)
}

func TestPolicyDatasourceRuleLookupFailureIsReturned(t *testing.T) {
	templatePolicyInvalid(t)
	d := newTemplateData(t, nil, nil)
	result, err := Datasource(d, "test", "https://example.invalid/", false)
	require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
	require.Nil(t, result)
}

func TestPolicyDatasourceTargetErrorsFailBeforeIO(t *testing.T) {
	templatePolicy(t, "source", "false", nil)
	d := newTemplateData(t, nil, nil)
	d.Sources["broken"] = &render.Source{Alias: "broken"}

	_, err := Datasource(d, "test", "%zz", false)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputInvalid)
	_, err = Datasource(d, "test", "broken", false)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	_, err = Datasource(d, "test", "https://example.invalid/", false, "a", "b")
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
}

func TestPolicyDatasourceUnguardedSchemeIsUnavailable(t *testing.T) {
	templatePolicy(t, "source", "false", nil)
	d := newTemplateData(t, nil, nil)
	result, err := Datasource(d, "test", "ftp://example.invalid/data.json", false)
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
			result, err := Datasource(d, "test", "cfg", tc.raw)
			require.NoError(t, err)
			require.Equal(t, tc.expected, result)
		})
	}

	t.Run("subpath beneath an allowed directory", func(t *testing.T) {
		d := newTemplateData(t, []string{"cfg=" + templateDatasourceURL(allowed) + "/"}, nil)
		result, err := Datasource(d, "test", "cfg", false, "config.toml")
		require.NoError(t, err)
		require.Equal(t, map[string]any{"k": "v"}, result)
	})

	t.Run("subpath escaping the allowed directory is denied", func(t *testing.T) {
		d := newTemplateData(t, []string{"cfg=" + templateDatasourceURL(allowed) + "/"}, nil)
		result, err := Datasource(d, "test", "cfg", false, filepath.Join("..", "secret.json"))
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
		require.Nil(t, result)
	})

	t.Run("file outside allowed roots is denied", func(t *testing.T) {
		d := newTemplateData(t, []string{"cfg=" + templateDatasourceURL(outsideFile)}, nil)
		result, err := Datasource(d, "test", "cfg", false)
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
		require.Nil(t, result)
		result, err = Datasource(d, "test", "cfg", true)
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
		require.Nil(t, result)
	})

	t.Run("unresolvable path fails closed", func(t *testing.T) {
		d := newTemplateData(t, []string{"cfg=" + templateDatasourceURL(filepath.Join(allowed, "missing.json"))}, nil)
		result, err := Datasource(d, "test", "cfg", false)
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
		require.Nil(t, result)
	})

	t.Run("missing file admitted by policy reports the native read error", func(t *testing.T) {
		templatePolicy(t, "source", "false", nil)
		d := newTemplateData(t, []string{"cfg=" + templateDatasourceURL(filepath.Join(allowed, "missing.json"))}, nil)
		result, err := Datasource(d, "test", "cfg", false)
		require.Error(t, err)
		require.NotErrorIs(t, err, errUtils.ErrPolicyDenied)
		require.Nil(t, result)
	})

	t.Run("malformed content fails to decode", func(t *testing.T) {
		d := newTemplateData(t, []string{"cfg=" + templateDatasourceURL(badJSON)}, nil)
		result, err := Datasource(d, "test", "cfg", false)
		require.Error(t, err)
		require.Nil(t, result)
	})

	t.Run("EJSON file is rejected unless read raw", func(t *testing.T) {
		d := newTemplateData(t, []string{"cfg=" + templateDatasourceURL(ejsonFile)}, nil)
		result, err := Datasource(d, "test", "cfg", false)
		require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
		require.Nil(t, result)
		result, err = Datasource(d, "test", "cfg", true)
		require.NoError(t, err)
		require.Equal(t, `{"_public_key":"abc"}`, result)
	})
}

func TestPolicyDatasourceEnvironment(t *testing.T) {
	templatePolicy(t, "source", "false", nil)
	t.Setenv("POLICY_DS_ENV", "from-environment")
	d := newTemplateData(t, []string{"envvar=env:POLICY_DS_ENV"}, nil)
	result, err := Datasource(d, "test", "envvar", false)
	require.NoError(t, err)
	require.Equal(t, "from-environment", result)
}

// An env datasource reads a variable, not a file, so a rule that denies files outside the allowed roots
// must admit it instead of describing the variable as a path in the working directory.
func TestPolicyDatasourceEnvironmentIsNotAFile(t *testing.T) {
	templatePolicy(t, "source", fileConfinementRule, []string{t.TempDir()})
	t.Setenv("POLICY_DS_ENV", "from-environment")
	d := newTemplateData(t, []string{"envvar=env:POLICY_DS_ENV"}, nil)
	result, err := Datasource(d, "test", "envvar", false)
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
		result, err := Datasource(d, "test", "api", false)
		require.NoError(t, err)
		require.Equal(t, map[string]any{"n": 3000000}, result)
		require.Equal(t, "Bearer token", gotAuth.Load())
	})

	t.Run("format is chosen from the path extension or content type", func(t *testing.T) {
		d := newTemplateData(t, nil, nil)
		yamlResult, err := Datasource(d, "test", server.URL+"/data.yaml", false)
		require.NoError(t, err)
		require.Equal(t, map[string]any{"a": "b"}, yamlResult)
		csvResult, err := Datasource(d, "test", server.URL+"/data.csv", false)
		require.NoError(t, err)
		require.Equal(t, [][]string{{"k", "v"}, {"1", "2"}}, csvResult)
	})

	t.Run("include returns the raw body", func(t *testing.T) {
		d := newTemplateData(t, nil, nil)
		result, err := Datasource(d, "test", server.URL+"/data.yaml", true)
		require.NoError(t, err)
		require.Equal(t, "a: b\n", result)
	})

	t.Run("subpath resolves against the base URL", func(t *testing.T) {
		d := newTemplateData(t, []string{"api=" + server.URL + "/"}, nil)
		result, err := Datasource(d, "test", "api", false, "data.yaml")
		require.NoError(t, err)
		require.Equal(t, map[string]any{"a": "b"}, result)
	})

	t.Run("redirects to an admitted location are followed", func(t *testing.T) {
		d := newTemplateData(t, nil, nil)
		result, err := Datasource(d, "test", server.URL+"/moved", false)
		require.NoError(t, err)
		require.Equal(t, map[string]any{"n": 3000000}, result)
	})

	t.Run("redirect loops stop after the limit", func(t *testing.T) {
		d := newTemplateData(t, nil, nil)
		result, err := Datasource(d, "test", server.URL+"/loop", false)
		require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
		require.Nil(t, result)
	})

	t.Run("non-200 status is unavailable", func(t *testing.T) {
		d := newTemplateData(t, nil, nil)
		result, err := Datasource(d, "test", server.URL+"/missing", false)
		require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
		require.ErrorContains(t, err, "404")
		require.Nil(t, result)
	})

	t.Run("body at the size limit is accepted", func(t *testing.T) {
		d := newTemplateData(t, nil, nil)
		result, err := Datasource(d, "test", server.URL+"/exact", true)
		require.NoError(t, err)
		require.Len(t, result, policyDatasourceMaxBytes)
	})

	t.Run("body over the size limit is rejected", func(t *testing.T) {
		d := newTemplateData(t, nil, nil)
		result, err := Datasource(d, "test", server.URL+"/huge", true)
		require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
		require.ErrorContains(t, err, "too large")
		require.Nil(t, result)
	})

	t.Run("EJSON response is rejected", func(t *testing.T) {
		d := newTemplateData(t, nil, nil)
		result, err := Datasource(d, "test", server.URL+"/ejson", false)
		require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
		require.Nil(t, result)
	})

	t.Run("denied host is never contacted", func(t *testing.T) {
		before := hits.Load()
		localhost := strings.Replace(server.URL, "127.0.0.1", "localhost", 1)
		d := newTemplateData(t, nil, nil)
		result, err := Datasource(d, "test", localhost+"/data.json", false)
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
		require.Nil(t, result)
		require.Equal(t, before, hits.Load())

		result, err = Datasource(d, "test", server.URL+"/data.json", false)
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

	// A render.Data without a context must still make the request.
	result, err := readPolicyHTTPDatasource(&render.Data{}, "test", target, nil, true)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	require.Nil(t, result)
}

func TestReadPolicyHTTPDatasourceRejectsUnbuildableRequest(t *testing.T) {
	target := &url.URL{Scheme: "http", Host: "bad host"}
	result, err := readPolicyHTTPDatasource(&render.Data{}, "test", target, nil, true)
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
	result, err := readPolicyHTTPDatasource(&render.Data{Ctx: ctx}, "test", target, nil, true)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	require.Nil(t, result)
	require.Zero(t, hits.Load())
}

func TestGuardedJSONKeepsIntegers(t *testing.T) {
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

	t.Run("EJSON rejection is unchanged", func(t *testing.T) {
		_, err := guardedTemplateJSON(`{"_public_key":"x"}`)
		require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
		_, err = parsePolicyJSONDatasource([]byte(`{"_public_key":"x"}`))
		require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	})
}

func TestDatasourceReachableWrapperPropagatesPolicyOutcomes(t *testing.T) {
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
	defer failing.Close()
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer healthy.Close()

	t.Run("wrapper propagates only genuine policy outcomes", func(t *testing.T) {
		templatePolicy(t, "source", "true", nil)
		probeFn, ok := policySourceFunction("datasourceReachable", nil, "", &render.Data{}).(func(string, ...string) (bool, error))
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
