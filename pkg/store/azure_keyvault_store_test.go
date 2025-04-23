package store

import (
	"context"
	"fmt"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockKeyVaultClient is a mock implementation of the Azure Key Vault client
type MockKeyVaultClient struct {
	mock.Mock
}

func (m *MockKeyVaultClient) SetSecret(ctx context.Context, name string, parameters azsecrets.SetSecretParameters, options *azsecrets.SetSecretOptions) (azsecrets.SetSecretResponse, error) {
	args := m.Called(ctx, name, parameters, options)
	return args.Get(0).(azsecrets.SetSecretResponse), args.Error(1)
}

func (m *MockKeyVaultClient) GetSecret(ctx context.Context, name string, version string, options *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error) {
	args := m.Called(ctx, name, version, options)
	return args.Get(0).(azsecrets.GetSecretResponse), args.Error(1)
}

func (m *MockKeyVaultClient) DeleteSecret(ctx context.Context, name string, options *azsecrets.DeleteSecretOptions) (azsecrets.DeleteSecretResponse, error) {
	args := m.Called(ctx, name, options)
	return args.Get(0).(azsecrets.DeleteSecretResponse), args.Error(1)
}

func (m *MockKeyVaultClient) NewListSecretPropertiesPager(options *azsecrets.ListSecretPropertiesOptions) *runtime.Pager[azsecrets.ListSecretPropertiesResponse] {
	args := m.Called(options)
	return args.Get(0).(*runtime.Pager[azsecrets.ListSecretPropertiesResponse])
}

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
		wantErr   bool
	}{
		{
			name:      "simple path",
			stack:     "dev",
			component: "app",
			key:       "config",
			expected:  "prefix-dev-app-config",
			wantErr:   false,
		},
		{
			name:      "empty stack",
			stack:     "",
			component: "app",
			key:       "config",
			expected:  "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := store.getKey(tt.stack, tt.component, tt.key)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestKeyVaultStore_InputValidation(t *testing.T) {
	mockClient := new(MockKeyVaultClient)
	delimiter := "-"
	store := &KeyVaultStore{
		client:         mockClient,
		prefix:         "prefix",
		stackDelimiter: &delimiter,
	}

	tests := []struct {
		name      string
		stack     string
		component string
		key       string
		value     interface{}
		operation string
		mockFn    func()
		wantError bool
	}{
		{
			name:      "empty stack",
			stack:     "",
			component: "app",
			key:       "config",
			value:     "test",
			operation: "set",
			mockFn:    func() {},
			wantError: true,
		},
		{
			name:      "empty component",
			stack:     "dev",
			component: "",
			key:       "config",
			value:     "test",
			operation: "set",
			mockFn:    func() {},
			wantError: true,
		},
		{
			name:      "empty key",
			stack:     "dev",
			component: "app",
			key:       "",
			value:     "test",
			operation: "set",
			mockFn:    func() {},
			wantError: true,
		},
		{
			name:      "non-string value",
			stack:     "dev",
			component: "app",
			key:       "config",
			value:     123,
			operation: "set",
			mockFn:    func() {},
			wantError: true,
		},
		{
			name:      "valid set operation",
			stack:     "dev",
			component: "app",
			key:       "config",
			value:     "test",
			operation: "set",
			mockFn: func() {
				mockClient.On("SetSecret", mock.Anything, "prefix-dev-app-config", mock.Anything, mock.Anything).
					Return(azsecrets.SetSecretResponse{}, nil)
			},
			wantError: false,
		},
		{
			name:      "valid get operation",
			stack:     "dev",
			component: "app",
			key:       "config",
			operation: "get",
			mockFn: func() {
				mockClient.On("GetSecret", mock.Anything, "prefix-dev-app-config", mock.Anything).
					Return("test-value", nil)
			},
			wantError: false,
		},
		{
			name:      "get operation error",
			stack:     "dev",
			component: "app",
			key:       "config",
			operation: "get",
			mockFn: func() {
				mockClient.On("GetSecret", mock.Anything, "prefix-dev-app-config", "", mock.Anything).
					Return(azsecrets.GetSecretResponse{}, fmt.Errorf("secret not found"))
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient.ExpectedCalls = nil
			mockClient.Calls = nil
			tt.mockFn()

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
			mockClient.AssertExpectations(t)
		})
	}
}

func TestKeyVaultStore_Set(t *testing.T) {
	mockClient := new(MockKeyVaultClient)
	delimiter := "-"
	store := &KeyVaultStore{
		client:         mockClient,
		prefix:         "prefix",
		stackDelimiter: &delimiter,
	}

	tests := []struct {
		name      string
		stack     string
		component string
		key       string
		value     interface{}
		mockFn    func()
		wantErr   bool
	}{
		{
			name:      "valid set",
			stack:     "dev",
			component: "app",
			key:       "config",
			value:     "test-value",
			mockFn: func() {
				params := azsecrets.SetSecretParameters{Value: stringPtr("test-value")}
				mockClient.On("SetSecret", mock.Anything, "prefix-dev-app-config", params, mock.Anything).
					Return(azsecrets.SetSecretResponse{}, nil)
			},
			wantErr: false,
		},
		{
			name:      "empty stack",
			stack:     "",
			component: "app",
			key:       "config",
			value:     "test",
			mockFn:    func() {},
			wantErr:   true,
		},
		{
			name:      "non-string value",
			stack:     "dev",
			component: "app",
			key:       "config",
			value:     123,
			mockFn:    func() {},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient.ExpectedCalls = nil
			mockClient.Calls = nil
			tt.mockFn()

			err := store.Set(tt.stack, tt.component, tt.key, tt.value)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			mockClient.AssertExpectations(t)
		})
	}
}

func TestKeyVaultStore_Get(t *testing.T) {
	mockClient := new(MockKeyVaultClient)
	delimiter := "-"
	store := &KeyVaultStore{
		client:         mockClient,
		prefix:         "prefix",
		stackDelimiter: &delimiter,
	}

	tests := []struct {
		name      string
		stack     string
		component string
		key       string
		mockFn    func()
		want      interface{}
		wantErr   bool
	}{
		{
			name:      "valid get",
			stack:     "dev",
			component: "app",
			key:       "config",
			mockFn: func() {
				value := "test-value"
				mockClient.On("GetSecret", mock.Anything, "prefix-dev-app-config", "", mock.Anything).
					Return(azsecrets.GetSecretResponse{Secret: azsecrets.Secret{Value: &value}}, nil)
			},
			want:    "test-value",
			wantErr: false,
		},
		{
			name:      "not found",
			stack:     "dev",
			component: "app",
			key:       "missing",
			mockFn: func() {
				mockClient.On("GetSecret", mock.Anything, "prefix-dev-app-missing", "", mock.Anything).
					Return(azsecrets.GetSecretResponse{}, fmt.Errorf("secret not found"))
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient.ExpectedCalls = nil
			mockClient.Calls = nil
			tt.mockFn()

			got, err := store.Get(tt.stack, tt.component, tt.key)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
			mockClient.AssertExpectations(t)
		})
	}
}

func TestKeyVaultStore_Delete(t *testing.T) {
	mockClient := new(MockKeyVaultClient)
	delimiter := "-"
	store := &KeyVaultStore{
		client:         mockClient,
		prefix:         "prefix",
		stackDelimiter: &delimiter,
	}

	tests := []struct {
		name      string
		stack     string
		component string
		key       string
		mockFn    func()
		wantErr   bool
	}{
		{
			name:      "valid delete",
			stack:     "dev",
			component: "app",
			key:       "config",
			mockFn: func() {
				mockClient.On("DeleteSecret", mock.Anything, "prefix-dev-app-config", mock.Anything).
					Return(azsecrets.DeleteSecretResponse{}, nil)
			},
			wantErr: false,
		},
		{
			name:      "not found",
			stack:     "dev",
			component: "app",
			key:       "missing",
			mockFn: func() {
				mockClient.On("DeleteSecret", mock.Anything, "prefix-dev-app-missing", mock.Anything).
					Return(azsecrets.DeleteSecretResponse{}, fmt.Errorf("secret not found"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient.ExpectedCalls = nil
			mockClient.Calls = nil
			tt.mockFn()

			err := store.Delete(tt.stack, tt.component, tt.key)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			mockClient.AssertExpectations(t)
		})
	}
}

func TestKeyVaultStore_List(t *testing.T) {
	mockClient := new(MockKeyVaultClient)
	delimiter := "-"
	store := &KeyVaultStore{
		client:         mockClient,
		prefix:         "prefix",
		stackDelimiter: &delimiter,
	}

	// Create test data
	id1 := azsecrets.ID("https://test.vault.azure.net/secrets/prefix-dev-app-config")
	id2 := azsecrets.ID("https://test.vault.azure.net/secrets/prefix-dev-app-secret")
	testSecrets := []*azsecrets.SecretProperties{
		{
			ID: &id1,
		},
		{
			ID: &id2,
		},
	}

	pager := &mockPager{
		pages: testSecrets,
	}

	mockClient.On("NewListSecretPropertiesPager", mock.Anything).Return(pager)

	tests := []struct {
		name      string
		stack     string
		component string
		expected  []string
		wantErr   bool
	}{
		{
			name:      "valid list",
			stack:     "dev",
			component: "app",
			expected:  []string{"config", "secret"},
			wantErr:   false,
		},
		{
			name:      "empty stack",
			stack:     "",
			component: "app",
			expected:  nil,
			wantErr:   true,
		},
		{
			name:      "empty component",
			stack:     "dev",
			component: "",
			expected:  nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := store.List(tt.stack, tt.component)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

type mockPager struct {
	pages    []*azsecrets.SecretProperties
	current  int
	finished bool
}

func (p *mockPager) More() bool {
	return !p.finished
}

func (p *mockPager) NextPage(ctx context.Context) (azsecrets.ListSecretPropertiesResponse, error) {
	if p.finished {
		return azsecrets.ListSecretPropertiesResponse{}, nil
	}

	response := azsecrets.ListSecretPropertiesResponse{
		SecretPropertiesListResult: azsecrets.SecretPropertiesListResult{
			Value: p.pages,
		},
	}
	p.finished = true
	return response, nil
}

func stringPtr(s string) *string {
	return &s
}
