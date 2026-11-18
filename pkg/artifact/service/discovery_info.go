package service

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/schema"
)

// discoveryInfoOf describes where a configured repository's discoverable data lives. Decoding the
// options is best effort: configuration was validated when the composition was built, and a
// repository whose options cannot be decoded here simply has no location, which discovery treats
// as not cacheable.
func discoveryInfoOf(cfg *schema.ArtifactRepositoryConfig) DiscoveryInfo {
	info := DiscoveryInfo{Kind: cfg.Kind, Catalog: cfg.Catalog, Identity: cfg.Identity, Options: cfg.Options}
	switch cfg.Kind {
	case artifact.RepositoryKindOCIRegistry:
		var options artifact.OCIRegistryOptions
		if artifact.DecodeOptions(cfg.Options, &options) == nil {
			info.Location = joinLocation(strings.ToLower(options.Registry), options.Prefix)
		}
	case artifact.RepositoryKindS3:
		var options artifact.S3Options
		if artifact.DecodeOptions(cfg.Options, &options) == nil {
			info.Location = "s3://" + joinLocation(options.Bucket, options.Prefix)
		}
	case artifact.RepositoryKindLocalDir, artifact.RepositoryKindLocalArchive:
		var options artifact.LocalDirOptions
		if artifact.DecodeOptions(cfg.Options, &options) == nil {
			info.Location = options.Path
		}
	case artifact.RepositoryKindOCILayout:
		var options artifact.OCILayoutOptions
		if artifact.DecodeOptions(cfg.Options, &options) == nil {
			info.Location = options.Path
		}
	}
	return info
}

// joinLocation joins a root and an optional slash-separated prefix.
func joinLocation(root, prefix string) string {
	prefix = strings.Trim(prefix, "/")
	if prefix == "" {
		return root
	}
	return root + "/" + prefix
}
