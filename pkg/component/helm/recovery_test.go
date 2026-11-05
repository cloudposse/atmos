package helm

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/cli"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"

	"github.com/cloudposse/atmos/pkg/policy/recovery"
	"github.com/cloudposse/atmos/pkg/policy/recovery/recoverytest"
)

type recoveryRoundTripper func(*http.Request) (*http.Response, error)

func (f recoveryRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestHelmRecoveryTransportRetriesPollWithoutRepeatingMutation(t *testing.T) {
	recoverytest.Install(t, "helm.apply.error", "input.error.code == '503'")
	runner, err := recovery.New(nil, "helm", "deploy")
	require.NoError(t, err)
	settings := cli.New()
	flags := settings.RESTClientGetter().(*genericclioptions.ConfigFlags)
	previousCalled := false
	flags.WrapConfigFn = func(config *rest.Config) *rest.Config {
		previousCalled = true
		return config
	}
	configureRecoveryTransport(settings, runner)
	config := flags.WrapConfigFn(&rest.Config{})
	require.True(t, previousCalled)
	mutations, polls := 0, 0
	transport := config.WrapTransport(recoveryRoundTripper(func(request *http.Request) (*http.Response, error) {
		status := http.StatusOK
		if request.Method == http.MethodPost {
			mutations++
		} else {
			polls++
			if polls == 1 {
				status = http.StatusServiceUnavailable
			}
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("response"))}, nil
	}))
	for _, method := range []string{http.MethodPost, http.MethodGet} {
		request, err := http.NewRequestWithContext(context.Background(), method, "https://cluster.invalid", nil)
		require.NoError(t, err)
		response, err := transport.RoundTrip(request)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, response.StatusCode)
		require.NoError(t, response.Body.Close())
	}
	require.Equal(t, 1, mutations)
	require.Equal(t, 2, polls)
}

func TestHelmRecoveryRepositoriesRetryOnlyFailedRepository(t *testing.T) {
	recoverytest.Install(t, "helm.apply.error", "input.error.message == 'transient'")
	runner, err := recovery.New(nil, "helm", "apply")
	require.NoError(t, err)
	original := setupRepositories
	t.Cleanup(func() { setupRepositories = original })
	var calls []string
	setupRepositories = func(repositories []chartRepository) error {
		require.Len(t, repositories, 1)
		calls = append(calls, repositories[0].Name)
		if len(calls) == 2 {
			return errors.New("transient")
		}
		return nil
	}
	require.NoError(t, setupRepositoriesWithRecovery(context.Background(), []chartRepository{{Name: "first"}, {Name: "second"}}, runner))
	require.Equal(t, []string{"first", "second", "second"}, calls)
}
