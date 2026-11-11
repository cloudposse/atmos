package exec

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/template/render"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestFuncMap(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	configAndStacksInfo := &schema.ConfigAndStacksInfo{}
	ctx := context.TODO()
	gomplateData := &render.Data{}

	fm := FuncMap(atmosConfig, configAndStacksInfo, ctx, gomplateData)

	// Verify the function map contains expected keys: "atmos" plus every
	// function templatefuncs.FuncMap() registers (e.g. "collectKeys").
	keys := u.StringKeysFromMap(fm)
	assert.ElementsMatch(t, []string{"atmos", "collectKeys"}, keys)

	// Verify the atmos function is callable and returns AtmosFuncs.
	atmosFunc, ok := fm["atmos"]
	assert.True(t, ok, "FuncMap should contain 'atmos' key")
	assert.NotNil(t, atmosFunc, "atmos function should not be nil")

	// Call the function to verify it returns AtmosFuncs instance.
	atmosFuncsInterface := atmosFunc.(func() any)()
	atmosFuncs, ok := atmosFuncsInterface.(*AtmosFuncs)
	assert.True(t, ok, "atmos function should return *AtmosFuncs")
	assert.NotNil(t, atmosFuncs, "AtmosFuncs should not be nil")

	// Verify AtmosFuncs has the expected configuration.
	assert.Equal(t, atmosConfig, atmosFuncs.atmosConfig)
	assert.Equal(t, configAndStacksInfo, atmosFuncs.configAndStacksInfo)
}

func TestAtmosFuncs_Component(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	configAndStacksInfo := &schema.ConfigAndStacksInfo{}
	ctx := context.TODO()
	gomplateData := &render.Data{}

	atmosFuncs := &AtmosFuncs{
		atmosConfig:         atmosConfig,
		configAndStacksInfo: configAndStacksInfo,
		ctx:                 ctx,
		gomplateData:        gomplateData,
	}

	// Test with empty parameters - should return error.
	_, err := atmosFuncs.Component("", "")
	assert.Error(t, err, "Component() should return error for empty parameters")
}

func TestAtmosFuncs_GomplateDatasource(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	configAndStacksInfo := &schema.ConfigAndStacksInfo{}
	ctx := context.TODO()
	gomplateData := &render.Data{}

	atmosFuncs := &AtmosFuncs{
		atmosConfig:         atmosConfig,
		configAndStacksInfo: configAndStacksInfo,
		ctx:                 ctx,
		gomplateData:        gomplateData,
	}

	// Test with invalid alias - should return error.
	_, err := atmosFuncs.GomplateDatasource("nonexistent-alias")
	assert.Error(t, err, "GomplateDatasource() should return error for invalid alias")

	// A render without datasources (Gomplate disabled) reads nothing, not even an absolute URL.
	withoutSources := &AtmosFuncs{atmosConfig: atmosConfig, configAndStacksInfo: configAndStacksInfo, ctx: ctx}
	_, err = withoutSources.GomplateDatasource("https://example.invalid/data.json")
	require.ErrorIs(t, err, errUtils.ErrGomplateDatasourceUnavailable)
}

// TestAtmosFuncs_GomplateDatasource_CacheKeyIncludesDefinition verifies that two stacks defining
// the same alias with different URLs each read their own value from the process-wide cache.
func TestAtmosFuncs_GomplateDatasource_CacheKeyIncludesDefinition(t *testing.T) {
	// No policy rules, so the read goes through the cache rather than the policy path.
	t.Setenv("ATMOS_POLICY_DIR", "")
	t.Chdir(t.TempDir())

	read := func(t *testing.T, content string) any {
		t.Helper()
		// An HTTP source keeps this portable: gomplate cannot read file:///C:/... URLs on Windows.
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/yaml")
			_, _ = io.WriteString(w, content)
		}))
		t.Cleanup(server.Close)

		sources := &render.Data{}
		_, err := sources.DefineDatasource("cache-key-config", server.URL+"/config.yaml")
		require.NoError(t, err)

		atmosFuncs := &AtmosFuncs{atmosConfig: &schema.AtmosConfiguration{}, ctx: context.TODO(), gomplateData: sources}
		value, err := atmosFuncs.GomplateDatasource("cache-key-config")
		require.NoError(t, err)
		return value.(map[string]any)["name"]
	}

	assert.Equal(t, "one", read(t, "name: one\n"))
	assert.Equal(t, "two", read(t, "name: two\n"))
}

func TestAtmosFuncs_Resolve(t *testing.T) {
	atmosFuncs := &AtmosFuncs{
		atmosConfig:         &schema.AtmosConfiguration{},
		configAndStacksInfo: &schema.ConfigAndStacksInfo{},
		ctx:                 context.TODO(),
		gomplateData:        &render.Data{},
	}

	t.Run("plain untagged string is returned unchanged", func(t *testing.T) {
		result, err := atmosFuncs.Resolve("just-a-string")
		require.NoError(t, err)
		assert.Equal(t, "just-a-string", result)
	})

	t.Run("resolves an !env YAML function at template time", func(t *testing.T) {
		t.Setenv("ATMOS_RESOLVE_TEST_VAR", "resolved-value")
		result, err := atmosFuncs.Resolve("!env ATMOS_RESOLVE_TEST_VAR")
		require.NoError(t, err)
		assert.Equal(t, "resolved-value", result)
	})

	t.Run("does not panic with nil configAndStacksInfo", func(t *testing.T) {
		funcs := &AtmosFuncs{
			atmosConfig:  &schema.AtmosConfiguration{},
			ctx:          context.TODO(),
			gomplateData: &render.Data{},
		}
		result, err := funcs.Resolve("plain")
		require.NoError(t, err)
		assert.Equal(t, "plain", result)
	})
}

// TestProcessTmplWithDatasources_GomplateDisabledHasNoDatasources verifies that with
// templates.settings.gomplate.enabled false, atmos.GomplateDatasource reads nothing, not even an
// absolute URL, and fails with the unavailable error instead. Sprig-only renders still work.
func TestProcessTmplWithDatasources_GomplateDisabledHasNoDatasources(t *testing.T) {
	// No policy rules, so the read would otherwise go through the process-wide cache.
	t.Setenv("ATMOS_POLICY_DIR", "")
	t.Chdir(t.TempDir())

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"name":"fetched"}`)
	}))
	t.Cleanup(server.Close)

	cfg := &schema.AtmosConfiguration{}
	cfg.Templates.Settings.Enabled = true
	cfg.Templates.Settings.Sprig.Enabled = true
	cfg.Templates.Settings.Gomplate.Enabled = false

	_, err := ProcessTmplWithDatasources(cfg, &schema.ConfigAndStacksInfo{}, schema.Settings{},
		"gomplate-disabled", `{{ (atmos.GomplateDatasource "`+server.URL+`/data.json").name }}`, nil, false)
	require.ErrorIs(t, err, errUtils.ErrGomplateDatasourceUnavailable)
	assert.Zero(t, requests.Load(), "no request is made when Gomplate is disabled")

	// Negative path: the rest of the render is unaffected.
	result, err := ProcessTmplWithDatasources(cfg, &schema.ConfigAndStacksInfo{}, schema.Settings{},
		"gomplate-disabled", `name: {{ upper "ok" }}`, nil, false)
	require.NoError(t, err)
	assert.Equal(t, "name: OK", result)
}

// TestProcessTmplWithDatasources_GlobalTimeoutIsKeptWhenTheStackSetsNone verifies the global
// templates.settings.gomplate.timeout applies when the stack manifest sets no timeout of its own,
// and that a stack-level timeout still overrides it.
func TestProcessTmplWithDatasources_GlobalTimeoutIsKeptWhenTheStackSetsNone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(1500 * time.Millisecond):
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"name":"slow"}`)
	}))
	t.Cleanup(server.Close)

	cfg := &schema.AtmosConfiguration{}
	cfg.Templates.Settings.Enabled = true
	cfg.Templates.Settings.Gomplate.Enabled = true
	cfg.Templates.Settings.Gomplate.Timeout = 1
	cfg.Templates.Settings.Gomplate.Datasources = map[string]schema.TemplatesSettingsGomplateDatasource{
		"slow": {Url: server.URL + "/data.json"},
	}
	text := `name: {{ (ds "slow").name }}`

	_, err := ProcessTmplWithDatasources(cfg, &schema.ConfigAndStacksInfo{}, schema.Settings{}, "timeout", text, nil, false)
	require.Error(t, err, "the global 1s timeout applies when the stack sets none")

	var stack schema.Settings
	stack.Templates.Settings.Gomplate.Timeout = 3
	result, err := ProcessTmplWithDatasources(cfg, &schema.ConfigAndStacksInfo{}, stack, "timeout", text, nil, false)
	require.NoError(t, err, "a stack-level timeout still overrides the global one")
	assert.Equal(t, "name: slow", result)
}

// TestNewPassScopeNormalizesDatasourceDefinitions verifies stack datasource definitions parse like
// render.Request.Datasources: a drive-letter file URL keeps the drive in its path instead of the
// host, and headers are kept.
func TestNewPassScopeNormalizesDatasourceDefinitions(t *testing.T) {
	settings := &schema.TemplatesSettings{}
	settings.Gomplate.Datasources = map[string]schema.TemplatesSettingsGomplateDatasource{
		"drive": {Url: "file://D:/data/a.json", Headers: map[string][]string{"X-Test": {"value"}}},
		"plain": {Url: "/abs/b.json"},
	}
	scope, err := newPassScope(&templatePass{templateSettings: settings}, true)
	require.NoError(t, err)
	t.Cleanup(scope.cancel)

	drive := scope.sources.Sources["drive"]
	require.NotNil(t, drive)
	assert.Empty(t, drive.URL.Host)
	assert.Equal(t, "D:/data/a.json", drive.URL.Path)
	assert.Equal(t, "value", drive.Header.Get("X-Test"))

	plain := scope.sources.Sources["plain"]
	require.NotNil(t, plain)
	assert.Equal(t, "file:///abs/b.json", plain.URL.String())

	// Negative path: an invalid definition fails the pass.
	settings.Gomplate.Datasources = map[string]schema.TemplatesSettingsGomplateDatasource{"bad": {Url: "%zz"}}
	_, err = newPassScope(&templatePass{templateSettings: settings}, true)
	require.ErrorIs(t, err, errUtils.ErrInvalidDatasourceURL)
}
