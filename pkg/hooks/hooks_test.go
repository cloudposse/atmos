package hooks

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
)

func TestGetHooks(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		info        *schema.ConfigAndStacksInfo
		wantErr     bool
		wantNilMap  bool
	}{
		{
			name:        "empty component and stack returns hooks with nil items",
			atmosConfig: &schema.AtmosConfiguration{},
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "",
				Stack:            "",
			},
			wantErr:    false,
			wantNilMap: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hooks, err := GetHooks(tt.atmosConfig, tt.info)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, hooks)
			assert.Equal(t, tt.atmosConfig, hooks.config)
			assert.Equal(t, tt.info, hooks.info)

			if tt.wantNilMap {
				assert.Nil(t, hooks.items)
			} else {
				assert.NotNil(t, hooks.items)
			}
		})
	}
}
