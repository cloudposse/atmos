package container

import (
	"context"

	"github.com/cloudposse/atmos/pkg/ci"
	ctr "github.com/cloudposse/atmos/pkg/container"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

var writeComponentStepSummary = ci.WriteStepSummary

func writeImageSummary(config *schema.AtmosConfiguration, info *ctr.ImageInfo, opts ctr.ImageSummaryOptions) {
	if !summaryEnabled(config) || info == nil {
		return
	}
	md := ctr.RenderImageSummaryMarkdown(info, opts)
	if md == "" {
		return
	}
	if err := writeComponentStepSummary(md); err != nil {
		log.Debug("container component: failed to write CI image summary", "image", opts.Image, "error", err)
	}
}

func inspectAndWriteImageSummary(ctx context.Context, runtime ctr.Runtime, config *schema.AtmosConfiguration, image, digest string) {
	if !summaryEnabled(config) || image == "" {
		return
	}
	info, err := runtime.ImageInspect(ctx, image)
	if err != nil {
		log.Debug("container component: failed to inspect image for CI summary", "image", image, "error", err)
		return
	}
	writeImageSummary(config, info, ctr.ImageSummaryOptions{Image: image, Digest: digest})
}

func summaryEnabled(config *schema.AtmosConfiguration) bool {
	if config == nil || !config.CI.Enabled {
		return false
	}
	return config.CI.Summary.Enabled == nil || *config.CI.Summary.Enabled
}

func firstNonEmpty(values []string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
