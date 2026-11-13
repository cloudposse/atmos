package main

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDescriptionMatchesManifestAndIndependentServices(t *testing.T) {
	got := description()
	digest := sha256.Sum256(manifestData)
	require.Equal(t, "sha256:"+hex.EncodeToString(digest[:]), got.ManifestDigest)
	require.Equal(t, "cloudposse/terragrunt", got.PluginName)
	// Preparation version 2 carries execution metadata, Terraform arguments, and
	// backend ownership. An earlier host only speaks version 1 and rejects it.
	versions := map[string]uint32{"component.discovery": 1, "component.validation": 1, "component.preparation": 2, "component.state": 1}
	services := []string{}
	for _, service := range got.Services {
		require.Equal(t, versions[service.Service], service.Min, service.Service)
		require.Equal(t, versions[service.Service], service.Max, service.Service)
		services = append(services, service.Service)
	}
	require.Equal(t, []string{"component.discovery", "component.validation", "component.preparation", "component.state"}, services)
	for _, export := range got.Exports {
		require.Equal(t, "terragrunt", export.Kind)
		require.Contains(t, services, export.Service)
	}
}
