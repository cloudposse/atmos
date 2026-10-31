package artifactwire_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifactwire"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewCatalogsComposesWithoutAccessingAnyRepository(t *testing.T) {
	config := &schema.AtmosConfiguration{Artifacts: schema.ArtifactConfig{Repositories: map[string]schema.ArtifactRepositoryConfig{
		"shared": {
			Kind: artifact.RepositoryKindOCIRegistry, Options: map[string]any{"registry": "127.0.0.1:1", "prefix": "acme/atmos"},
			Catalog: "oci://127.0.0.1:1/acme/atmos/catalog:stable",
		},
	}}}
	maintainer, err := artifactwire.NewCatalogs(config)
	require.NoError(t, err, "building the service performs no registry access")
	require.NotNil(t, maintainer)
}

func TestNewCatalogsRejectsInvalidConfiguration(t *testing.T) {
	config := &schema.AtmosConfiguration{Artifacts: schema.ArtifactConfig{Repositories: map[string]schema.ArtifactRepositoryConfig{
		"shared": {Kind: artifact.RepositoryKindOCIRegistry, Options: map[string]any{"registry": "127.0.0.1:1"}, Catalog: "https://example.com/catalog"},
	}}}
	_, err := artifactwire.NewCatalogs(config)
	require.Error(t, err)
}
