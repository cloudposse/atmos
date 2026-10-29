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
	services := []string{}
	for _, service := range got.Services {
		require.EqualValues(t, 1, service.Min)
		require.EqualValues(t, 1, service.Max)
		services = append(services, service.Service)
	}
	require.Equal(t, []string{"component.discovery", "component.validation", "component.preparation", "component.state"}, services)
	for _, export := range got.Exports {
		require.Equal(t, "terragrunt", export.Kind)
		require.Contains(t, services, export.Service)
	}
}
