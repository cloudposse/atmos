package oci

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/service"
	"github.com/cloudposse/atmos/pkg/perf"
)

// pluginPolicy is the plugin-format immutability policy.
type pluginPolicy struct{}

var _ service.ImmutabilityPolicy = pluginPolicy{}

// PluginImmutability returns the [service.ImmutabilityPolicy] that keeps legacy Atmos plugin
// releases immutable. Register it with service.WithImmutabilityPolicy. The service consults it for
// the stored artifact and for the incoming one, so a generic publication or copy can neither
// overwrite a plugin release nor turn an existing generic artifact into one, with or without
// force.
//
// A candidate is a plugin release when its original OCI root carries the plugin release
// annotations, when a layer or config uses the plugin media types, or when an incoming prepared
// artifact stages a plugin bundle or labels itself with the plugin namespace. The repositories
// enforce the same rule on their own writes, so this seam and the storage layer agree.
func PluginImmutability() service.ImmutabilityPolicy {
	defer perf.Track(nil, "oci.PluginImmutability")()

	return pluginPolicy{}
}

// Immutable implements [service.ImmutabilityPolicy].
func (pluginPolicy) Immutable(candidate service.Candidate) (bool, string) {
	defer perf.Track(nil, "oci.pluginPolicy.Immutable")()

	if candidate.Inspection != nil {
		if immutable, why := inspectionIsPlugin(candidate.Inspection); immutable {
			return true, why
		}
	}
	if candidate.Prepared != nil {
		return preparedIsPlugin(candidate.Prepared)
	}
	return false, ""
}

// inspectionIsPlugin checks an inspected artifact.
func inspectionIsPlugin(inspection *artifact.Inspection) (bool, string) {
	if original := inspection.Representation.OriginalRoot; original != nil {
		if hasPluginAnnotations(original.Annotations) {
			return true, "legacy plugin release " + original.Annotations[pluginAnnotationName] + " " + original.Annotations[pluginAnnotationVersion]
		}
	}
	for index := range inspection.Representation.Payload {
		if inspection.Representation.Payload[index].MediaType == pluginBundleMediaType {
			return true, "legacy plugin release (plugin bundle layer)"
		}
	}
	if record := inspection.Record; record != nil && hasPluginLabel(record.Metadata.Labels) {
		return true, "plugin-format artifact (plugin labels)"
	}
	return false, ""
}

// preparedIsPlugin checks an incoming prepared artifact.
func preparedIsPlugin(prepared *artifact.PreparedArtifact) (bool, string) {
	for index := range prepared.Blobs {
		if prepared.Blobs[index].Descriptor.MediaType == pluginBundleMediaType {
			return true, "plugin-format artifact (plugin bundle blob)"
		}
	}
	if hasPluginLabel(prepared.Record.Metadata.Labels) {
		return true, "plugin-format artifact (plugin labels)"
	}
	return false, ""
}

func hasPluginLabel(labels map[string]string) bool {
	for key := range labels {
		if strings.HasPrefix(key, pluginPrefix) {
			return true
		}
	}
	return false
}
