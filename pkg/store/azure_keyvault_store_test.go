package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewKeyVaultStore(t *testing.T) {
	tests := []struct {
		name      string
		options   KeyVaultStoreOptions
		wantError bool
	}{
		{
			name: "valid options",
			options: KeyVaultStoreOptions{
				VaultURL: "https://test-vault.vault.azure.net/",
			},
			wantError: false,
		},
		{
			name: "missing vault url",
			options: KeyVaultStoreOptions{
				VaultURL: "",
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewKeyVaultStore(tt.options)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestKeyVaultStore_getKey(t *testing.T) {
	delimiter := "-"
	store := &KeyVaultStore{
		prefix:         "prefix",
		stackDelimiter: &delimiter,
	}

	tests := []struct {
		name      string
		stack     string
		component string
		key       string
		expected  string
	}{
		{
			name:      "simple path",
			stack:     "dev",
			component: "app",
			key:       "config",
			expected:  "prefix-dev-app-config",
		},
		{
			name:      "nested component",
			stack:     "dev",
			component: "app/service",
			key:       "config",
			expected:  "prefix-dev-app-service-config",
		},
		{
			name:      "multi-level stack",
			stack:     "dev-us-west-2",
			component: "app",
			key:       "config",
			expected:  "prefix-dev-us-west-2-app-config",
		},
		{
			name:      "uppercase characters",
			stack:     "Dev",
			component: "App/Service",
			key:       "Config",
			expected:  "prefix-dev-app-service-config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := store.getKey(tt.stack, tt.component, tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestKeyVaultStore_InputValidation(t *testing.T) {
	store := &KeyVaultStore{
		prefix:         "prefix",
		stackDelimiter: new(string),
	}

	tests := []struct {
		name      string
		stack     string
		component string
		key       string
		value     interface{}
		operation string
		wantError bool
	}{
		{
			name:      "empty stack",
			stack:     "",
			component: "app",
			key:       "config",
			value:     "test",
			operation: "set",
			wantError: true,
		},
		{
			name:      "empty component",
			stack:     "dev",
			component: "",
			key:       "config",
			value:     "test",
			operation: "set",
			wantError: true,
		},
		{
			name:      "empty key",
			stack:     "dev",
			component: "app",
			key:       "",
			value:     "test",
			operation: "set",
			wantError: true,
		},
		{
			name:      "non-string value",
			stack:     "dev",
			component: "app",
			key:       "config",
			value:     123,
			operation: "set",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			if tt.operation == "set" {
				err = store.Set(tt.stack, tt.component, tt.key, tt.value)
			} else {
				_, err = store.Get(tt.stack, tt.component, tt.key)
			}

			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
