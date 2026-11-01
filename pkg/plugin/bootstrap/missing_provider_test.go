package bootstrap

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestMissingProviderNamesTheProviderAndHowToSupplyIt(t *testing.T) {
	err := missingProviderError([]ComponentReference{{Provider: "terragrunt", Capability: "component.discovery"}, {Provider: "terragrunt", Capability: "component.preparation"}})
	require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension)
	details := strings.Join(errUtils.AllDetails(err), "\n")
	assert.Contains(t, details, "components.terraform.terragrunt.enabled")
	assert.Equal(t, 1, strings.Count(details, "terragrunt` configuration provider"), "a provider is named once however many capabilities were requested")
	hints := strings.Join(errUtils.AllHints(err), "\n")
	assert.Contains(t, hints, "plugins.packages")
	assert.Contains(t, hints, "ATMOS_PROFILE")
}
