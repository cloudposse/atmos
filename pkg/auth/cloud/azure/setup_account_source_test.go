package azure

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/auth/types"
)

func TestAccountSourceForAuthMethod(t *testing.T) {
	// Mirrors what az itself records per flow.
	assert.Equal(t, "authorization_code", accountSourceForAuthMethod(types.AzureAuthMethodInteractive))
	assert.Equal(t, "device_code", accountSourceForAuthMethod(types.AzureAuthMethodDeviceCode))
	assert.Equal(t, "device_code", accountSourceForAuthMethod(""))
}
