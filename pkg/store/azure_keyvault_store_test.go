package store

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"github.com/stretchr/testify/assert"
)

type mockClient struct {
	getSecretFunc func(ctx context.Context, name string, version string, options *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error)
	setSecretFunc func(ctx context.Context, name string, parameters azsecrets.SetSecretParameters, options *azsecrets.SetSecretOptions) (azsecrets.SetSecretResponse, error)
}

func (m *mockClient) GetSecret(ctx context.Context, name string, version string, options *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error) {
	return m.getSecretFunc(ctx, name, version, options)
}

func (m *mockClient) SetSecret(ctx context.Context, name string, parameters azsecrets.SetSecretParameters, options *azsecrets.SetSecretOptions) (azsecrets.SetSecretResponse, error) {
	return m.setSecretFunc(ctx, name, parameters, options)
}

func TestKeyVaultStore_Set(t *testing.T) {
	tests := []struct {
		name      string
		stack     string
		component string
		key       string
		value     interface{}
		mockFunc  func(ctx context.Context, name string, parameters azsecrets.SetSecretParameters, options *azsecrets.SetSecretOptions) (azsecrets.SetSecretResponse, error)
		wantErr   error
	}{
		{
			name:      "success",
			stack:     "dev",
			component: "app",
			key:       "secret",
			value:     "value",
			mockFunc: func(ctx context.Context, name string, parameters azsecrets.SetSecretParameters, options *azsecrets.SetSecretOptions) (azsecrets.SetSecretResponse, error) {
				return azsecrets.SetSecretResponse{}, nil
			},
		},
		{
			name:      "empty stack",
			stack:     "",
			component: "app",
			key:       "secret",
			value:     "value",
			wantErr:   ErrEmptyStack,
		},
		{
			name:      "empty component",
			stack:     "dev",
			component: "",
			key:       "secret",
			value:     "value",
			wantErr:   ErrEmptyComponent,
		},
		{
			name:      "empty key",
			stack:     "dev",
			component: "app",
			key:       "",
			value:     "value",
			wantErr:   ErrEmptyKey,
		},
		{
			name:      "non-string value",
			stack:     "dev",
			component: "app",
			key:       "secret",
			value:     123,
			wantErr:   ErrValueMustBeString,
		},
		{
			name:      "permission denied",
			stack:     "dev",
			component: "app",
			key:       "secret",
			value:     "value",
			mockFunc: func(ctx context.Context, name string, parameters azsecrets.SetSecretParameters, options *azsecrets.SetSecretOptions) (azsecrets.SetSecretResponse, error) {
				return azsecrets.SetSecretResponse{}, &azcore.ResponseError{StatusCode: statusCodeForbidden}
			},
			wantErr: ErrPermissionDenied,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockClient{
				setSecretFunc: tt.mockFunc,
			}
			store := &KeyVaultStore{
				client:         client,
				vaultURL:       "https://test.vault.azure.net",
				stackDelimiter: stringPtr("-"),
			}

			err := store.Set(tt.stack, tt.component, tt.key, tt.value)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestKeyVaultStore_Get(t *testing.T) {
	tests := []struct {
		name      string
		stack     string
		component string
		key       string
		mockFunc  func(ctx context.Context, name string, version string, options *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error)
		want      interface{}
		wantErr   error
	}{
		{
			name:      "success",
			stack:     "dev",
			component: "app",
			key:       "secret",
			mockFunc: func(ctx context.Context, name string, version string, options *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error) {
				value := "test-value"
				return azsecrets.GetSecretResponse{
					Secret: azsecrets.Secret{
						Value: &value,
					},
				}, nil
			},
			want: "test-value",
		},
		{
			name:      "empty stack",
			stack:     "",
			component: "app",
			key:       "secret",
			wantErr:   ErrEmptyStack,
		},
		{
			name:      "empty component",
			stack:     "dev",
			component: "",
			key:       "secret",
			wantErr:   ErrEmptyComponent,
		},
		{
			name:      "empty key",
			stack:     "dev",
			component: "app",
			key:       "",
			wantErr:   ErrEmptyKey,
		},
		{
			name:      "not found",
			stack:     "dev",
			component: "app",
			key:       "secret",
			mockFunc: func(ctx context.Context, name string, version string, options *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error) {
				return azsecrets.GetSecretResponse{}, &azcore.ResponseError{StatusCode: statusCodeNotFound}
			},
			wantErr: ErrResourceNotFound,
		},
		{
			name:      "permission denied",
			stack:     "dev",
			component: "app",
			key:       "secret",
			mockFunc: func(ctx context.Context, name string, version string, options *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error) {
				return azsecrets.GetSecretResponse{}, &azcore.ResponseError{StatusCode: statusCodeForbidden}
			},
			wantErr: ErrPermissionDenied,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockClient{
				getSecretFunc: tt.mockFunc,
			}
			store := &KeyVaultStore{
				client:         client,
				vaultURL:       "https://test.vault.azure.net",
				stackDelimiter: stringPtr("-"),
			}

			got, err := store.Get(tt.stack, tt.component, tt.key)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func stringPtr(s string) *string {
	return &s
}
