package providers

import (
	"context"
	"errors"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	storepkg "github.com/cloudposse/atmos/pkg/store"
	"github.com/stretchr/testify/assert"
)

// errTestBackend is a generic (non-*azcore.ResponseError) backend error used to exercise the
// fallback error-wrapping path.
var errTestBackend = errors.New("backend failure")

type mockClient struct {
	getSecretFunc    func(ctx context.Context, name string, version string, options *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error)
	setSecretFunc    func(ctx context.Context, name string, parameters azsecrets.SetSecretParameters, options *azsecrets.SetSecretOptions) (azsecrets.SetSecretResponse, error)
	deleteSecretFunc func(ctx context.Context, name string, options *azsecrets.DeleteSecretOptions) (azsecrets.DeleteSecretResponse, error)
	// listVersionsFunc returns the single page yielded by the versions pager, letting tests
	// simulate existence (nil error) or not-found/permission errors without a secret value.
	listVersionsFunc func(name string) (azsecrets.ListSecretPropertiesVersionsResponse, error)
}

func (m *mockClient) GetSecret(ctx context.Context, name string, version string, options *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error) {
	return m.getSecretFunc(ctx, name, version, options)
}

func (m *mockClient) SetSecret(ctx context.Context, name string, parameters azsecrets.SetSecretParameters, options *azsecrets.SetSecretOptions) (azsecrets.SetSecretResponse, error) {
	return m.setSecretFunc(ctx, name, parameters, options)
}

func (m *mockClient) DeleteSecret(ctx context.Context, name string, options *azsecrets.DeleteSecretOptions) (azsecrets.DeleteSecretResponse, error) {
	return m.deleteSecretFunc(ctx, name, options)
}

// NewListSecretPropertiesVersionsPager builds a real runtime.Pager backed by listVersionsFunc so
// the mock exercises the same NextPage code path the store uses, returning metadata only.
func (m *mockClient) NewListSecretPropertiesVersionsPager(name string, _ *azsecrets.ListSecretPropertiesVersionsOptions) *runtime.Pager[azsecrets.ListSecretPropertiesVersionsResponse] {
	return runtime.NewPager(runtime.PagingHandler[azsecrets.ListSecretPropertiesVersionsResponse]{
		More: func(azsecrets.ListSecretPropertiesVersionsResponse) bool {
			return false
		},
		Fetcher: func(_ context.Context, _ *azsecrets.ListSecretPropertiesVersionsResponse) (azsecrets.ListSecretPropertiesVersionsResponse, error) {
			return m.listVersionsFunc(name)
		},
	})
}

func TestAzureKeyVaultStore_Set(t *testing.T) {
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
			name:      "success with string",
			stack:     "dev",
			component: "app",
			key:       "secret",
			value:     "value",
			mockFunc: func(ctx context.Context, name string, parameters azsecrets.SetSecretParameters, options *azsecrets.SetSecretOptions) (azsecrets.SetSecretResponse, error) {
				return azsecrets.SetSecretResponse{}, nil
			},
		},
		{
			name:      "success with map",
			stack:     "dev",
			component: "app",
			key:       "config",
			value:     map[string]interface{}{"key": "value"},
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
			wantErr:   storepkg.ErrEmptyStack,
		},
		{
			name:      "empty component",
			stack:     "dev",
			component: "",
			key:       "secret",
			value:     "value",
			wantErr:   storepkg.ErrEmptyComponent,
		},
		{
			name:      "empty key",
			stack:     "dev",
			component: "app",
			key:       "",
			value:     "value",
			wantErr:   storepkg.ErrEmptyKey,
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
			wantErr: storepkg.ErrPermissionDenied,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockClient{
				setSecretFunc: tt.mockFunc,
			}
			store := &AzureKeyVaultStore{
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

func TestAzureKeyVaultStore_Get(t *testing.T) {
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
			name:      "success with string",
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
			name:      "success with JSON",
			stack:     "dev",
			component: "app",
			key:       "config",
			mockFunc: func(ctx context.Context, name string, version string, options *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error) {
				value := `{"key":"value","number":123}`
				return azsecrets.GetSecretResponse{
					Secret: azsecrets.Secret{
						Value: &value,
					},
				}, nil
			},
			want: map[string]interface{}{"key": "value", "number": float64(123)},
		},
		{
			name:      "empty stack",
			stack:     "",
			component: "app",
			key:       "secret",
			wantErr:   storepkg.ErrEmptyStack,
		},
		{
			name:      "empty component",
			stack:     "dev",
			component: "",
			key:       "secret",
			wantErr:   storepkg.ErrEmptyComponent,
		},
		{
			name:      "empty key",
			stack:     "dev",
			component: "app",
			key:       "",
			wantErr:   storepkg.ErrEmptyKey,
		},
		{
			name:      "not found",
			stack:     "dev",
			component: "app",
			key:       "secret",
			mockFunc: func(ctx context.Context, name string, version string, options *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error) {
				return azsecrets.GetSecretResponse{}, &azcore.ResponseError{StatusCode: statusCodeNotFound}
			},
			wantErr: storepkg.ErrResourceNotFound,
		},
		{
			name:      "permission denied",
			stack:     "dev",
			component: "app",
			key:       "secret",
			mockFunc: func(ctx context.Context, name string, version string, options *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error) {
				return azsecrets.GetSecretResponse{}, &azcore.ResponseError{StatusCode: statusCodeForbidden}
			},
			wantErr: storepkg.ErrPermissionDenied,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockClient{
				getSecretFunc: tt.mockFunc,
			}
			store := &AzureKeyVaultStore{
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

func TestAzureKeyVaultStore_Delete(t *testing.T) {
	tests := []struct {
		name      string
		stack     string
		component string
		key       string
		mockFunc  func(ctx context.Context, name string, options *azsecrets.DeleteSecretOptions) (azsecrets.DeleteSecretResponse, error)
		wantErr   error
	}{
		{
			name:      "success",
			stack:     "dev",
			component: "app",
			key:       "secret",
			mockFunc: func(ctx context.Context, name string, options *azsecrets.DeleteSecretOptions) (azsecrets.DeleteSecretResponse, error) {
				return azsecrets.DeleteSecretResponse{}, nil
			},
		},
		{
			name:      "empty stack",
			stack:     "",
			component: "app",
			key:       "secret",
			wantErr:   storepkg.ErrEmptyStack,
		},
		{
			name:      "empty component",
			stack:     "dev",
			component: "",
			key:       "secret",
			wantErr:   storepkg.ErrEmptyComponent,
		},
		{
			name:      "empty key",
			stack:     "dev",
			component: "app",
			key:       "",
			wantErr:   storepkg.ErrEmptyKey,
		},
		{
			name:      "not found",
			stack:     "dev",
			component: "app",
			key:       "secret",
			mockFunc: func(ctx context.Context, name string, options *azsecrets.DeleteSecretOptions) (azsecrets.DeleteSecretResponse, error) {
				return azsecrets.DeleteSecretResponse{}, &azcore.ResponseError{StatusCode: statusCodeNotFound}
			},
			wantErr: storepkg.ErrResourceNotFound,
		},
		{
			name:      "permission denied",
			stack:     "dev",
			component: "app",
			key:       "secret",
			mockFunc: func(ctx context.Context, name string, options *azsecrets.DeleteSecretOptions) (azsecrets.DeleteSecretResponse, error) {
				return azsecrets.DeleteSecretResponse{}, &azcore.ResponseError{StatusCode: statusCodeForbidden}
			},
			wantErr: storepkg.ErrPermissionDenied,
		},
		{
			name:      "generic error wrapped as delete failure",
			stack:     "dev",
			component: "app",
			key:       "secret",
			mockFunc: func(ctx context.Context, name string, options *azsecrets.DeleteSecretOptions) (azsecrets.DeleteSecretResponse, error) {
				return azsecrets.DeleteSecretResponse{}, errTestBackend
			},
			wantErr: storepkg.ErrDeleteSecret,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockClient{
				deleteSecretFunc: tt.mockFunc,
			}
			store := &AzureKeyVaultStore{
				client:         client,
				vaultURL:       "https://test.vault.azure.net",
				stackDelimiter: stringPtr("-"),
			}

			err := store.Delete(tt.stack, tt.component, tt.key)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAzureKeyVaultStore_Has(t *testing.T) {
	tests := []struct {
		name     string
		mockFunc func(name string) (azsecrets.ListSecretPropertiesVersionsResponse, error)
		want     bool
		wantErr  error
	}{
		{
			name: "present",
			mockFunc: func(name string) (azsecrets.ListSecretPropertiesVersionsResponse, error) {
				// A secret with at least one version exists; the value is never part of this response.
				id := azsecrets.ID("https://test.vault.azure.net/secrets/" + name + "/v1")
				return azsecrets.ListSecretPropertiesVersionsResponse{
					SecretPropertiesListResult: azsecrets.SecretPropertiesListResult{
						Value: []*azsecrets.SecretProperties{{ID: &id}},
					},
				}, nil
			},
			want: true,
		},
		{
			name: "absent",
			mockFunc: func(string) (azsecrets.ListSecretPropertiesVersionsResponse, error) {
				return azsecrets.ListSecretPropertiesVersionsResponse{}, &azcore.ResponseError{StatusCode: statusCodeNotFound}
			},
			want: false,
		},
		{
			name: "other error propagated",
			mockFunc: func(string) (azsecrets.ListSecretPropertiesVersionsResponse, error) {
				return azsecrets.ListSecretPropertiesVersionsResponse{}, &azcore.ResponseError{StatusCode: statusCodeForbidden}
			},
			want:    false,
			wantErr: storepkg.ErrPermissionDenied,
		},
		{
			name: "generic error wrapped as access failure",
			mockFunc: func(string) (azsecrets.ListSecretPropertiesVersionsResponse, error) {
				return azsecrets.ListSecretPropertiesVersionsResponse{}, errTestBackend
			},
			want:    false,
			wantErr: storepkg.ErrAccessSecret,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockClient{
				listVersionsFunc: tt.mockFunc,
				// Has() must never retrieve the secret value: fail loudly if GetSecret is reached.
				getSecretFunc: func(context.Context, string, string, *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error) {
					t.Fatal("Has() must not call GetSecret; existence is checked via the versions pager")
					return azsecrets.GetSecretResponse{}, nil
				},
			}
			store := &AzureKeyVaultStore{
				client:         client,
				vaultURL:       "https://test.vault.azure.net",
				stackDelimiter: stringPtr("-"),
			}

			got, err := store.Has("dev", "app", "secret")
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestAzureKeyVaultStore_HasValidatesInput confirms Has() validates its arguments before touching
// the client, matching Get/Set/Delete.
func TestAzureKeyVaultStore_HasValidatesInput(t *testing.T) {
	store := &AzureKeyVaultStore{
		client:         &mockClient{},
		vaultURL:       "https://test.vault.azure.net",
		stackDelimiter: stringPtr("-"),
	}

	tests := []struct {
		name      string
		stack     string
		component string
		key       string
		wantErr   error
	}{
		{name: "empty stack", stack: "", component: "app", key: "secret", wantErr: storepkg.ErrEmptyStack},
		{name: "empty component", stack: "dev", component: "", key: "secret", wantErr: storepkg.ErrEmptyComponent},
		{name: "empty key", stack: "dev", component: "app", key: "", wantErr: storepkg.ErrEmptyKey},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := store.Has(tt.stack, tt.component, tt.key)
			assert.False(t, got)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestAzureKeyVaultStore_normalizeSecretName(t *testing.T) {
	store := &AzureKeyVaultStore{}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple name",
			input:    "simple-name",
			expected: "simple-name",
		},
		{
			name:     "with slashes",
			input:    "path/to/secret",
			expected: "path-to-secret",
		},
		{
			name:     "with special characters",
			input:    "secret@#$%^&*()",
			expected: "secret",
		},
		{
			name:     "with spaces",
			input:    "secret name with spaces",
			expected: "secret-name-with-spaces",
		},
		{
			name:     "with multiple hyphens",
			input:    "secret--name---with----hyphens",
			expected: "secret-name-with-hyphens",
		},
		{
			name:     "with leading/trailing hyphens",
			input:    "-secret-name-",
			expected: "secret-name",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "default",
		},
		{
			name:     "only special characters",
			input:    "@#$%^&*()",
			expected: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := store.normalizeSecretName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func stringPtr(s string) *string {
	return &s
}

func TestAzureKeyVaultStore_GetKey(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		mockValue     *string
		mockError     error
		expectedValue interface{}
		expectError   bool
		errorContains string
	}{
		{
			name:          "successful string retrieval",
			key:           "app-settings",
			mockValue:     stringPtr("production"),
			mockError:     nil,
			expectedValue: "production",
			expectError:   false,
		},
		{
			name:          "successful JSON object retrieval",
			key:           "database-config",
			mockValue:     stringPtr(`{"host":"localhost","port":5432}`),
			mockError:     nil,
			expectedValue: map[string]interface{}{"host": "localhost", "port": float64(5432)},
			expectError:   false,
		},
		{
			name:          "successful JSON array retrieval",
			key:           "server-list",
			mockValue:     stringPtr(`["server1","server2","server3"]`),
			mockError:     nil,
			expectedValue: []interface{}{"server1", "server2", "server3"},
			expectError:   false,
		},
		{
			name:          "secret not found",
			key:           "nonexistent",
			mockValue:     nil,
			mockError:     &azcore.ResponseError{StatusCode: 404},
			expectedValue: nil,
			expectError:   true,
			errorContains: "resource not found",
		},
		{
			name:          "empty secret value",
			key:           "empty-secret",
			mockValue:     stringPtr(""),
			mockError:     nil,
			expectedValue: "",
			expectError:   false,
		},
		{
			name:          "malformed JSON returns as string",
			key:           "invalid-json",
			mockValue:     stringPtr(`{"invalid": json`),
			mockError:     nil,
			expectedValue: `{"invalid": json`,
			expectError:   false,
		},
		{
			name:          "permission denied error",
			key:           "restricted-secret",
			mockValue:     nil,
			mockError:     &azcore.ResponseError{StatusCode: 403},
			expectedValue: nil,
			expectError:   true,
			errorContains: "permission denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			mockClient := &mockClient{
				getSecretFunc: func(ctx context.Context, name string, version string, options *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error) {
					normalizedKey := (&AzureKeyVaultStore{}).normalizeSecretName(tt.key)
					assert.Equal(t, normalizedKey, name)

					if tt.mockError != nil {
						return azsecrets.GetSecretResponse{}, tt.mockError
					}

					return azsecrets.GetSecretResponse{
						Secret: azsecrets.Secret{
							Value: tt.mockValue,
						},
					}, nil
				},
			}

			store := &AzureKeyVaultStore{
				client:         mockClient,
				prefix:         "myapp",
				stackDelimiter: stringPtr("/"),
			}

			// Act
			result, err := store.GetKey(tt.key)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Equal(t, tt.expectedValue, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedValue, result)
			}
		})
	}
}

func TestNewAzureKeyVaultStore_EmptyVaultURL(t *testing.T) {
	_, err := NewAzureKeyVaultStore(AzureKeyVaultStoreOptions{}, "")
	assert.ErrorIs(t, err, storepkg.ErrVaultURLRequired)
}

func TestAzureKeyVaultStore_getKey_NilDelimiter(t *testing.T) {
	s := &AzureKeyVaultStore{vaultURL: "https://test.vault.azure.net"} // nil delimiter.
	_, err := s.getKey("dev", "app", "k")
	assert.ErrorIs(t, err, storepkg.ErrStackDelimiterNotSet)
}

func TestAzureKeyVaultStore_Set_MoreErrors(t *testing.T) {
	t.Run("nil value", func(t *testing.T) {
		s := &AzureKeyVaultStore{client: &mockClient{}, vaultURL: "https://v", stackDelimiter: stringPtr("-")}
		assert.ErrorIs(t, s.Set("dev", "app", "k", nil), storepkg.ErrNilValue)
	})

	t.Run("marshal error", func(t *testing.T) {
		s := &AzureKeyVaultStore{client: &mockClient{}, vaultURL: "https://v", stackDelimiter: stringPtr("-")}
		assert.ErrorIs(t, s.Set("dev", "app", "k", make(chan int)), storepkg.ErrSerializeJSON)
	})

	t.Run("getKey error on nil delimiter", func(t *testing.T) {
		s := &AzureKeyVaultStore{client: &mockClient{}, vaultURL: "https://v"} // nil delimiter.
		assert.ErrorIs(t, s.Set("dev", "app", "k", "v"), storepkg.ErrGetKey)
	})

	t.Run("generic set error", func(t *testing.T) {
		client := &mockClient{
			setSecretFunc: func(_ context.Context, _ string, _ azsecrets.SetSecretParameters, _ *azsecrets.SetSecretOptions) (azsecrets.SetSecretResponse, error) {
				return azsecrets.SetSecretResponse{}, errors.New("set boom")
			},
		}
		s := &AzureKeyVaultStore{client: client, vaultURL: "https://v", stackDelimiter: stringPtr("-")}
		assert.ErrorIs(t, s.Set("dev", "app", "k", "v"), storepkg.ErrSetParameter)
	})
}

func TestAzureKeyVaultStore_Get_MoreCases(t *testing.T) {
	t.Run("getKey error on nil delimiter", func(t *testing.T) {
		s := &AzureKeyVaultStore{client: &mockClient{}, vaultURL: "https://v"} // nil delimiter.
		_, err := s.Get("dev", "app", "k")
		assert.ErrorIs(t, err, storepkg.ErrGetKey)
	})

	t.Run("generic access error", func(t *testing.T) {
		client := &mockClient{
			getSecretFunc: func(_ context.Context, _ string, _ string, _ *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error) {
				return azsecrets.GetSecretResponse{}, errors.New("get boom")
			},
		}
		s := &AzureKeyVaultStore{client: client, vaultURL: "https://v", stackDelimiter: stringPtr("-")}
		_, err := s.Get("dev", "app", "k")
		assert.ErrorIs(t, err, storepkg.ErrAccessSecret)
	})

	t.Run("nil value returns empty string", func(t *testing.T) {
		client := &mockClient{
			getSecretFunc: func(_ context.Context, _ string, _ string, _ *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error) {
				return azsecrets.GetSecretResponse{}, nil // Value is nil.
			},
		}
		s := &AzureKeyVaultStore{client: client, vaultURL: "https://v", stackDelimiter: stringPtr("-")}
		v, err := s.Get("dev", "app", "k")
		assert.NoError(t, err)
		assert.Equal(t, "", v)
	})
}

func TestAzureKeyVaultStore_GetKey_MoreCases(t *testing.T) {
	t.Run("empty key", func(t *testing.T) {
		s := &AzureKeyVaultStore{client: &mockClient{}, vaultURL: "https://v", stackDelimiter: stringPtr("-")}
		_, err := s.GetKey("")
		assert.ErrorIs(t, err, storepkg.ErrEmptyKey)
	})

	t.Run("generic access error", func(t *testing.T) {
		client := &mockClient{
			getSecretFunc: func(_ context.Context, _ string, _ string, _ *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error) {
				return azsecrets.GetSecretResponse{}, errors.New("get boom")
			},
		}
		s := &AzureKeyVaultStore{client: client, vaultURL: "https://v", stackDelimiter: stringPtr("-")}
		_, err := s.GetKey("k")
		assert.ErrorIs(t, err, storepkg.ErrAccessSecret)
	})

	t.Run("nil value returns empty string", func(t *testing.T) {
		client := &mockClient{
			getSecretFunc: func(_ context.Context, _ string, _ string, _ *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error) {
				return azsecrets.GetSecretResponse{}, nil // Value is nil.
			},
		}
		s := &AzureKeyVaultStore{client: client, vaultURL: "https://v", stackDelimiter: stringPtr("-")}
		v, err := s.GetKey("k")
		assert.NoError(t, err)
		assert.Equal(t, "", v)
	})
}

func TestBuildAzureKeyVaultStore_ParseError(t *testing.T) {
	_, err := buildAzureKeyVaultStore("n", storepkg.StoreConfig{
		Options: map[string]interface{}{"prefix": []string{"x"}},
	})
	assert.ErrorIs(t, err, storepkg.ErrParseAzureKeyVaultOptions)
}
