package devcontainer

import (
	"context"

	"github.com/cloudposse/atmos/pkg/container"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
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

	// Build image if build configuration is specified.
	// This must happen before createContainer since it sets config.Image.
	// Track whether we built the image to skip pulling in that case.
	builtLocally := p.config.Build != nil
	if err := buildImageIfNeeded(p.ctx, p.runtime, p.config, p.name); err != nil {
		return err
	}

	// Pull latest image unless --no-pull is set or image was built locally.
	// Locally built images don't exist in remote registries, so pulling would fail.
	if !builtLocally {
		if err := pullImageIfNeeded(p.ctx, p.runtime, p.config.Image, p.noPull); err != nil {
			return err
		}
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

	_ = ui.Successf("Container %s rebuilt successfully", p.containerName)

	// Inspect container to get actual port information after rebuild.
	containerInfo, err := p.runtime.Inspect(p.ctx, containerID)
	if err != nil {
		log.Warn("Failed to inspect container for port info", "error", err)
		displayContainerInfo(p.config, nil)
	} else {
		displayContainerInfo(p.config, containerInfo)
	}

	return nil
}
