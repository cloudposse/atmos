package azure

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/auth/types"
)

func TestAccountSourceForAuthMethod(t *testing.T) {
	// The label mirrors what az itself records per flow.
	tests := []struct {
		name       string
		authMethod string
		want       string
	}{
		{
			name:       "interactive maps to authorization_code",
			authMethod: types.AzureAuthMethodInteractive,
			want:       "authorization_code",
		},
		{
			name:       "device code keeps device_code",
			authMethod: types.AzureAuthMethodDeviceCode,
			want:       "device_code",
		},
		{
			name:       "unset defaults to device_code",
			authMethod: "",
			want:       "device_code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, accountSourceForAuthMethod(tt.authMethod))
		})
	}
}
