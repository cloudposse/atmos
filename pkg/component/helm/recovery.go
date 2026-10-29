package helm

import (
	"context"
	"net/http"
	"os"

	"helm.sh/helm/v4/pkg/cli"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"

	"github.com/cloudposse/atmos/pkg/policy/recovery"
)

func configureRecoveryTransport(settings *cli.EnvSettings, runner *recovery.Runner) {
	if runner == nil {
		return
	}
	flags, ok := settings.RESTClientGetter().(*genericclioptions.ConfigFlags)
	if !ok {
		return
	}
	previous := flags.WrapConfigFn
	flags.WrapConfigFn = func(config *rest.Config) *rest.Config {
		if previous != nil {
			config = previous(config)
		}
		config = rest.CopyConfig(config)
		config.Wrap(func(base http.RoundTripper) http.RoundTripper {
			return &recovery.Transport{Base: base, Runner: runner}
		})
		return config
	}
}

func newRecoveryActionContext(namespace string, runner *recovery.Runner) (*actionContext, error) {
	actx, err := newActionContext(namespace)
	if err != nil || runner == nil {
		return actx, err
	}
	configureRecoveryTransport(actx.settings, runner)
	// Init rebuilds the lazy Kubernetes clients; it performs no release operation.
	err = actx.cfg.Init(actx.settings.RESTClientGetter(), namespace, os.Getenv("HELM_DRIVER")) //nolint:forbidigo // Helm's existing driver selection.
	return actx, err
}

func setupRepositoriesWithRecovery(ctx context.Context, repositories []chartRepository, runner *recovery.Runner) error {
	if runner == nil {
		return setupRepositories(repositories)
	}
	for i := range repositories {
		if err := recovery.Do(ctx, runner, "repository", func() error { return setupRepositories(repositories[i : i+1]) }); err != nil {
			return err
		}
	}
	return nil
}
