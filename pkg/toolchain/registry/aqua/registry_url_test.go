package aqua

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegistryBaseURL_Default(t *testing.T) {
	t.Setenv("ATMOS_TOOLCHAIN_AQUA_REGISTRY_URL", "")

	assert.Equal(t, "https://raw.githubusercontent.com/aquaproj/aqua-registry/main", RegistryBaseURL())
}

func TestRegistryBaseURL_Override(t *testing.T) {
	t.Setenv("ATMOS_TOOLCHAIN_AQUA_REGISTRY_URL", "https://mirror.corp.example.com/aqua-registry/")

	assert.Equal(t, "https://mirror.corp.example.com/aqua-registry", RegistryBaseURL())
}
