package plugin

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/plugin/bootstrap"
	"github.com/cloudposse/atmos/pkg/plugin/install"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
)

// errMissingConfiguration explains a command that ran before Atmos configuration was loaded.
func errMissingConfiguration() error {
	return errUtils.Build(errUtils.ErrPluginInvalidConfig).
		WithExplanation("Atmos configuration is not loaded.").
		WithHint("Run this command from a project directory that contains an `atmos.yaml`, or pass `--config`.").Err()
}

func newManager(command *cobra.Command) (*manage.Manager, error) {
	if atmosConfig == nil {
		return nil, errMissingConfiguration()
	}
	files, _ := command.Flags().GetStringSlice("config")
	override, err := cfg.ResolveConfigOverride(files)
	if err != nil {
		return nil, err
	}
	file, err := cfg.ResolveEditableConfigFile(atmosConfig, override)
	if errors.Is(err, cfg.ErrNoEditableConfig) {
		file = filepath.Join(bootstrap.ProjectDirectory(atmosConfig), cfg.AtmosConfigFileName)
	} else if err != nil {
		return nil, err
	}
	registry, err := bootstrap.Descriptors()
	if err != nil {
		return nil, err
	}
	service, err := install.NewService(atmosConfig, registry, install.WithRegistryAuthPreparation(prepareRegistryAuth))
	if err != nil {
		return nil, err
	}
	return &manage.Manager{Project: filepath.Dir(file), ConfigFile: filepath.Base(file), Config: atmosConfig, Installer: service}, nil
}

func prepareRegistryAuth(ctx context.Context) (func(), error) {
	return bootstrap.PrepareAvailable(ctx, atmosConfig)
}
