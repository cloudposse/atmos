package devcontainer

import (
	"context"

	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// Rebuild rebuilds a devcontainer from scratch.
func (m *Manager) Rebuild(atmosConfig *schema.AtmosConfiguration, name, instance, identityName string, noPull bool) error {
	defer perf.Track(atmosConfig, "devcontainer.Rebuild")()

	config, settings, err := m.configLoader.LoadConfig(atmosConfig, name)
	if err != nil {
		return err
	}

	// Inject identity environment variables if identity is specified.
	if identityName != "" {
		ctx := context.Background()
		if err := m.identityManager.InjectIdentityEnvironment(ctx, config, identityName); err != nil {
			return err
		}
	}

	runtime, err := m.runtimeDetector.DetectRuntime(settings.Runtime)
	if err != nil {
		return err
	}

	ctx := context.Background()
	containerName, err := GenerateContainerName(name, instance)
	if err != nil {
		return err
	}

	params := &rebuildParams{
		ctx:           ctx,
		runtime:       runtime,
		config:        config,
		containerName: containerName,
		name:          name,
		instance:      instance,
		noPull:        noPull,
	}
	return rebuildContainer(params)
}

// rebuildParams holds parameters for rebuilding a container.
type rebuildParams struct {
	ctx           context.Context
	runtime       container.Runtime
	config        *Config
	containerName string
	name          string
	instance      string
	noPull        bool
}

// rebuildContainer stops, removes, and recreates a container.
func rebuildContainer(p *rebuildParams) error {
	// Stop and remove existing container if it exists.
	if err := stopAndRemoveContainer(p.ctx, p.runtime, p.containerName); err != nil {
		return err
	}

	// Pull latest image unless --no-pull is set.
	if err := pullImageIfNeeded(p.ctx, p.runtime, p.config.Image, p.noPull); err != nil {
		return err
	}

	// Create and start new container.
	params := &containerParams{
		ctx:           p.ctx,
		runtime:       p.runtime,
		config:        p.config,
		containerName: p.containerName,
		name:          p.name,
		instance:      p.instance,
	}
	containerID, err := createContainer(params)
	if err != nil {
		return err
	}

	if err := startContainer(p.ctx, p.runtime, containerID, p.containerName); err != nil {
		return err
	}

	u.PrintfMessageToTUI("%s Container %s rebuilt successfully\n", theme.Styles.Checkmark.String(), p.containerName)
	return nil
}
