package step

import (
	"context"

	"github.com/cloudposse/atmos/pkg/container"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

func writeContainerImageSummary(config *schema.AtmosConfiguration, info *container.ImageInfo, opts container.ImageSummaryOptions) {
	if !containerSummaryEnabled(config) || info == nil {
		return
	}
	md := container.RenderImageSummaryMarkdown(info, opts)
	if md == "" {
		return
	}
	if err := writeStepSummaryFn(md); err != nil {
		log.Debug("container step: failed to write CI image summary", "image", opts.Image, "error", err)
	}
}

func writePushedImageSummaries(ctx context.Context, runtime container.Runtime, config *schema.AtmosConfiguration, pushes []*container.PushResult) {
	if !containerSummaryEnabled(config) {
		return
	}
	for _, pushed := range pushes {
		if pushed == nil || pushed.Image == "" {
			continue
		}
		info, err := runtime.ImageInspect(ctx, pushed.Image)
		if err != nil {
			log.Debug("container step: failed to inspect pushed image for CI summary", "image", pushed.Image, "error", err)
			continue
		}
		writeContainerImageSummary(config, info, container.ImageSummaryOptions{
			Image:  pushed.Image,
			Digest: pushed.Digest,
		})
	}
}

func containerSummaryEnabled(config *schema.AtmosConfiguration) bool {
	if config == nil || !config.CI.Enabled {
		return false
	}
	return config.CI.Summary.Enabled == nil || *config.CI.Summary.Enabled
}
