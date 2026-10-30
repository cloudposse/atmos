package exec

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/policy/admission"
	"github.com/cloudposse/atmos/pkg/policy/guard"
	"github.com/cloudposse/atmos/pkg/schema"
	data "github.com/cloudposse/atmos/pkg/template/gomplate"
)

// failPolicyEnabled replaces the capability lookup with one that fails when shouldFail reports true for the
// kind and the 1-based call number. Other calls report the capability as enabled. The production lookup is
// restored when the test ends.
func failPolicyEnabled(t *testing.T, shouldFail func(kind string, call int) bool) {
	t.Helper()

	var calls atomic.Int32
	guard.SetEnabled(func(kind string) (bool, error) {
		if shouldFail(kind, int(calls.Add(1))) {
			return false, fmt.Errorf("%w: capability lookup failed", errUtils.ErrPolicyEvaluation)
		}
		return true, nil
	})
	t.Cleanup(func() {
		guard.SetEnabled(func(kind string) (bool, error) { return admission.HasRules(policy.InputKind(kind)) })
	})
}

func TestPolicyTemplateFuncsWithoutRulesReturnsOriginalMap(t *testing.T) {
	setups := map[string]func(t *testing.T){
		"no policy directory configured": func(t *testing.T) { t.Setenv("ATMOS_POLICY_DIR", "") },
		// Rules exist, but for a capability that template functions never consult.
		"rules only for other capabilities": func(t *testing.T) { templatePolicy(t, "command", "false", nil) },
	}
	for name, setup := range setups {
		t.Run(name, func(t *testing.T) {
			setup(t)
			original := template.FuncMap{"upper": strings.ToUpper}

			result, err := policyTemplateFuncs(original, "stack.yaml", &data.Data{})
			require.NoError(t, err)
			assert.Equal(t, reflect.ValueOf(original).Pointer(), reflect.ValueOf(result).Pointer(), "the original map is returned unwrapped")
			assert.Equal(t, reflect.ValueOf(strings.ToUpper).Pointer(), reflect.ValueOf(result["upper"]).Pointer())

			rendered, err := ProcessTmpl(&schema.AtmosConfiguration{}, "test", `{{ upper "ok" }}-{{ env "POLICY_UNSET_VARIABLE" }}`, nil, false)
			require.NoError(t, err)
			assert.Equal(t, "OK-", rendered)
		})
	}
}

func TestPolicyTemplateFuncsWrapsEveryFunctionWhenRulesAreEnabled(t *testing.T) {
	for _, kind := range []string{"function", "source"} {
		t.Run(kind, func(t *testing.T) {
			templatePolicy(t, kind, "false", nil)
			original := template.FuncMap{"upper": strings.ToUpper, "answer": func() int { return 42 }}

			result, err := policyTemplateFuncs(original, "stack.yaml", &data.Data{})
			require.NoError(t, err)
			require.Len(t, result, len(original))
			assert.NotEqual(t, reflect.ValueOf(original).Pointer(), reflect.ValueOf(result).Pointer(), "a private map is returned, never the cached one")
			assert.Equal(t, reflect.ValueOf(strings.ToUpper).Pointer(), reflect.ValueOf(original["upper"]).Pointer(), "the cached map is not mutated")

			upper, ok := result["upper"].(func(string) (string, error))
			require.True(t, ok, "single-result functions gain an error result: %T", result["upper"])
			value, err := upper("abc")
			require.NoError(t, err)
			assert.Equal(t, "ABC", value)

			answer, ok := result["answer"].(func() (int, error))
			require.True(t, ok, "%T", result["answer"])
			number, err := answer()
			require.NoError(t, err)
			assert.Equal(t, 42, number)
		})
	}
}

func TestPolicyTemplateFuncsCapabilityLookupErrors(t *testing.T) {
	original := template.FuncMap{"upper": strings.ToUpper}
	tests := []struct {
		name       string
		shouldFail func(kind string, call int) bool
	}{
		{"function lookup", func(kind string, _ int) bool { return kind == "function" }},
		{"source lookup", func(kind string, _ int) bool { return kind == "source" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			failPolicyEnabled(t, tc.shouldFail)
			result, err := policyTemplateFuncs(original, "stack.yaml", &data.Data{})
			require.ErrorIs(t, err, errUtils.ErrPolicyEvaluation)
			assert.Nil(t, result, "a failed lookup must not hand back unguarded functions")
		})
	}
}

func TestTemplateRenderersPropagateCapabilityLookupErrors(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	config.Templates.Settings.Enabled = true
	config.Templates.Settings.Gomplate.Enabled = true

	t.Run("ProcessTmpl", func(t *testing.T) {
		failPolicyEnabled(t, func(string, int) bool { return true })
		rendered, err := ProcessTmpl(&schema.AtmosConfiguration{}, "test", `{{ add 1 2 }}`, nil, false)
		require.ErrorIs(t, err, errUtils.ErrPolicyEvaluation)
		assert.Empty(t, rendered)
	})

	t.Run("ProcessTmplWithDatasources", func(t *testing.T) {
		failPolicyEnabled(t, func(string, int) bool { return true })
		rendered, err := ProcessTmplWithDatasources(config, &schema.ConfigAndStacksInfo{}, schema.Settings{}, "test", `{{ add 1 2 }}`, map[string]any{}, false)
		require.ErrorIs(t, err, errUtils.ErrPolicyEvaluation)
		assert.Empty(t, rendered)
	})

	gomplateTests := []struct {
		name       string
		shouldFail func(kind string, call int) bool
	}{
		{"function lookup", func(kind string, _ int) bool { return kind == "function" }},
		{"source lookup", func(kind string, _ int) bool { return kind == "source" }},
		// The first two lookups decide whether to guard; the third happens inside the function-map wrapper.
		{"wrapper lookup", func(_ string, call int) bool { return call >= 3 }},
	}
	for _, tc := range gomplateTests {
		t.Run("ProcessTmplWithDatasourcesGomplate "+tc.name, func(t *testing.T) {
			failPolicyEnabled(t, tc.shouldFail)
			rendered, err := ProcessTmplWithDatasourcesGomplate(config, "test", `{{ add 1 2 }}`, map[string]any{}, false)
			require.ErrorIs(t, err, errUtils.ErrPolicyEvaluation)
			assert.Empty(t, rendered)
		})
	}
}

func TestTemplateGetHostByNameSourceAdmission(t *testing.T) {
	templatePolicy(t, "source", "input.scheme == 'dns' && input.host == 'blocked.example.invalid'", nil)

	t.Run("denied host", func(t *testing.T) {
		err := admitTemplateCall("getHostByName", "stack.yaml", []reflect.Value{reflect.ValueOf("blocked.example.invalid")})
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)

		rendered, err := ProcessTmpl(&schema.AtmosConfiguration{}, "test", `{{ getHostByName "blocked.example.invalid" }}`, nil, false)
		require.ErrorContains(t, err, "policy")
		assert.Empty(t, rendered)
	})

	t.Run("host the rule does not match is admitted", func(t *testing.T) {
		err := admitTemplateCall("getHostByName", "stack.yaml", []reflect.Value{reflect.ValueOf("allowed.example.invalid")})
		require.NoError(t, err)
	})

	t.Run("other functions never consult the DNS source gate", func(t *testing.T) {
		err := admitTemplateCall("upper", "stack.yaml", []reflect.Value{reflect.ValueOf("blocked.example.invalid")})
		require.NoError(t, err)
	})

	t.Run("a call without arguments has no host to check", func(t *testing.T) {
		require.NoError(t, admitTemplateCall("getHostByName", "stack.yaml", nil))
	})
}

func TestAtmosFuncsFunctionAdmission(t *testing.T) {
	file := filepath.Join(t.TempDir(), "stack.yaml")
	require.NoError(t, os.WriteFile(file, []byte("stack"), 0o600))
	newFuncs := func() AtmosFuncs {
		return AtmosFuncs{
			atmosConfig:  &schema.AtmosConfiguration{},
			ctx:          context.Background(),
			gomplateData: &data.Data{},
			sourceFile:   file,
		}
	}

	t.Run("denied atmos functions fail before doing any work", func(t *testing.T) {
		templatePolicy(t, "function", "input.engine == 'go-template' && input.name.startsWith('atmos.')", nil)
		funcs := newFuncs()
		// A loaded manifest would otherwise produce a different error, proving admission runs first.
		funcs.manifestLoadFile = file

		result, err := funcs.Component("vpc", "dev")
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
		assert.Nil(t, result)
		assert.NotErrorIs(t, err, errUtils.ErrComponentFuncDuringManifestLoad)

		result, err = funcs.GomplateDatasource("config")
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
		assert.Nil(t, result)

		result, err = funcs.Store("store", "dev", "vpc", "key")
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
		assert.Nil(t, result)

		result, err = funcs.Resolve("!env POLICY_TEST_SECRET")
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
		assert.Nil(t, result)
	})

	t.Run("admitted calls run the underlying function", func(t *testing.T) {
		templatePolicy(t, "function", functionAllowAllRule, nil)
		funcs := newFuncs()

		_, err := funcs.GomplateDatasource("no-such-alias")
		require.Error(t, err, "an unknown datasource is an ordinary error")
		assert.NotErrorIs(t, err, errUtils.ErrPolicyDenied)

		funcs.manifestLoadFile = file
		_, err = funcs.Component("vpc", "dev")
		require.ErrorIs(t, err, errUtils.ErrComponentFuncDuringManifestLoad)
		assert.NotErrorIs(t, err, errUtils.ErrPolicyDenied)
	})
}

func TestAtmosFuncsResolveRemembersRemoteProvenance(t *testing.T) {
	guard.ResetOrigins()
	t.Cleanup(guard.ResetOrigins)
	templatePolicy(t, "function", functionAllowAllRule, nil)
	file := filepath.Join(t.TempDir(), "stack.yaml")
	require.NoError(t, os.WriteFile(file, []byte("stack"), 0o600))
	const input = "value-resolved-from-a-remote-manifest"

	local := AtmosFuncs{atmosConfig: &schema.AtmosConfiguration{}, ctx: context.Background(), sourceFile: file}
	resolved, err := local.Resolve(input)
	require.NoError(t, err)
	assert.Equal(t, input, resolved, "an untagged string is returned unchanged")
	assert.Equal(t, "local", guard.ValueOrigin(input).Provenance)

	// A value seen from a remote manifest keeps the more restrictive provenance through later lookups.
	guard.RegisterRemote(file, "https://example.com/stack.yaml")
	remoteInput := input + "-remote"
	resolved, err = local.Resolve(remoteInput)
	require.NoError(t, err)
	assert.Equal(t, remoteInput, resolved)
	assert.Equal(t, "remote", guard.ValueOrigin(remoteInput).Provenance)
	assert.Equal(t, "unknown", guard.ValueOrigin("never-resolved-value").Provenance)
}

// Without a source rule nothing guards the read, so repeated template calls must keep using the
// per-alias cache instead of fetching the datasource again. With a source rule every call is checked.
func TestAtmosFuncsGomplateDatasourceCaching(t *testing.T) {
	var hits atomic.Int32
	var heads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			heads.Add(1)
		} else {
			hits.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"n":1}`))
	}))
	t.Cleanup(server.Close)
	funcs := func(t *testing.T, alias string) AtmosFuncs {
		t.Helper()
		t.Cleanup(func() { gomplateDatasourceFuncSyncMap.Delete(alias) })
		d, err := data.NewData([]string{alias + "=" + server.URL + "/data.json"}, nil)
		require.NoError(t, err)
		return AtmosFuncs{atmosConfig: &schema.AtmosConfiguration{}, ctx: context.Background(), gomplateData: d, sourceFile: "stack.yaml"}
	}

	t.Run("no source rule reuses the alias cache", func(t *testing.T) {
		templatePolicy(t, "function", "false", nil)
		hits.Store(0)
		heads.Store(0)

		// Each template render builds its own datasource set, so only the shared alias cache can avoid a refetch.
		first, err := funcs(t, "cached-alias").GomplateDatasource("cached-alias")
		require.NoError(t, err)
		second, err := funcs(t, "cached-alias").GomplateDatasource("cached-alias")
		require.NoError(t, err)

		require.Equal(t, first, second)
		require.EqualValues(t, 1, hits.Load(), "the second render is served from the cache")
		require.EqualValues(t, 1, heads.Load(), "Gomplate v4 reads metadata once, then reuses the alias cache")
	})

	t.Run("a source rule checks and reads every call", func(t *testing.T) {
		templatePolicy(t, "source", "false", nil)
		hits.Store(0)

		for range 2 {
			result, err := funcs(t, "guarded-alias").GomplateDatasource("guarded-alias")
			require.NoError(t, err)
			require.Equal(t, map[string]any{"n": 1}, result)
		}

		require.EqualValues(t, 2, hits.Load(), "guarded reads are not cached across renders")
	})

	t.Run("a rule lookup failure is returned", func(t *testing.T) {
		templatePolicyInvalid(t)
		f := funcs(t, "invalid-alias")

		result, err := f.GomplateDatasource("invalid-alias")

		require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
		require.Nil(t, result)
	})
}
