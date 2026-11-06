package exec

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	data "github.com/cloudposse/atmos/pkg/template/gomplate"
)

// A datasource probe must return configuration failures instead of reporting an unreachable source.
func TestDatasourceReachableRejectsEmptyPolicyDirectory(t *testing.T) {
	templatePolicyInvalid(t)
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	probe, ok := policySourceFunction("datasourceReachable", nil, "", &data.Data{}).(func(string, ...string) (bool, error))
	require.True(t, ok)

	reachable, err := probe(server.URL)

	require.ErrorIs(t, err, errUtils.ErrNoPoliciesConfigured)
	require.False(t, reachable)
	require.Zero(t, hits.Load(), "policy configuration is checked before contacting the source")
}

// Redirect guards must preserve configuration errors through the HTTP client's error wrapping.
func TestDatasourceRedirectRejectsEmptyPolicyDirectory(t *testing.T) {
	templatePolicyInvalid(t)
	var redirects, blocked atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		redirects.Add(1)
		http.Redirect(w, r, "/blocked", http.StatusFound)
	})
	mux.HandleFunc("/blocked", func(w http.ResponseWriter, _ *http.Request) {
		blocked.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	target, err := url.Parse(server.URL + "/start")
	require.NoError(t, err)

	result, err := readPolicyHTTPDatasource(&data.Data{}, "test", target, nil, true)

	require.ErrorIs(t, err, errUtils.ErrNoPoliciesConfigured)
	require.NotErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	require.Nil(t, result)
	require.EqualValues(t, 1, redirects.Load())
	require.Zero(t, blocked.Load(), "the redirect target must not be contacted")
}
