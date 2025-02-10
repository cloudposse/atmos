package store

import (
	"context"
	"errors"
	"os"
	"testing"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/googleapis/gax-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockGSMClient is a mock implementation of secretmanager.Client
type MockGSMClient struct {
	mock.Mock
}

func (m *MockGSMClient) CreateSecret(ctx context.Context, req *secretmanagerpb.CreateSecretRequest, opts ...gax.CallOption) (*secretmanagerpb.Secret, error) {
	args := m.Called(mock.Anything, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*secretmanagerpb.Secret), args.Error(1)
}

func (m *MockGSMClient) AddSecretVersion(ctx context.Context, req *secretmanagerpb.AddSecretVersionRequest, opts ...gax.CallOption) (*secretmanagerpb.SecretVersion, error) {
	args := m.Called(mock.Anything, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*secretmanagerpb.SecretVersion), args.Error(1)
}

func (m *MockGSMClient) AccessSecretVersion(ctx context.Context, req *secretmanagerpb.AccessSecretVersionRequest, opts ...gax.CallOption) (*secretmanagerpb.AccessSecretVersionResponse, error) {
	args := m.Called(mock.Anything, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*secretmanagerpb.AccessSecretVersionResponse), args.Error(1)
}

func (m *MockGSMClient) Close() error {
	args := m.Called()
	return args.Error(0)
}

func TestGSMStore_Set(t *testing.T) {
	testPrefix := "test-prefix"
	testDelimiter := "-"

	tests := []struct {
		name      string
		stack     string
		component string
		key       string
		value     interface{}
		mockFn    func(*MockGSMClient)
		wantErr   bool
	}{
		{
			name:      "successful set",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "config-key",
			value:     "test-value",
			mockFn: func(m *MockGSMClient) {
				m.On("CreateSecret", mock.Anything, mock.MatchedBy(func(req *secretmanagerpb.CreateSecretRequest) bool {
					expectedReq := &secretmanagerpb.CreateSecretRequest{
						Parent:   "projects/test-project",
						SecretId: "test-prefix_dev-usw2_app_service_config-key",
						Secret: &secretmanagerpb.Secret{
							Replication: &secretmanagerpb.Replication{
								Replication: &secretmanagerpb.Replication_Automatic_{
									Automatic: &secretmanagerpb.Replication_Automatic{},
								},
							},
						},
					}
					return req.Parent == expectedReq.Parent &&
						req.SecretId == "test-prefix_dev_usw2_app_service_config-key" &&
						req.Secret.GetReplication().GetAutomatic() != nil
				})).Return(&secretmanagerpb.Secret{
					Name: "projects/test-project/secrets/test-prefix_dev_usw2_app_service_config-key",
				}, nil)

				m.On("AddSecretVersion", mock.Anything, mock.MatchedBy(func(req *secretmanagerpb.AddSecretVersionRequest) bool {
					expectedReq := &secretmanagerpb.AddSecretVersionRequest{
						Parent: "projects/test-project/secrets/test-prefix_dev_usw2_app_service_config-key",
						Payload: &secretmanagerpb.SecretPayload{
							Data: []byte("test-value"),
						},
					}
					return req.Parent == expectedReq.Parent &&
						string(req.Payload.Data) == string(expectedReq.Payload.Data)
				})).Return(&secretmanagerpb.SecretVersion{}, nil)
			},
			wantErr: false,
		},
		{
			name:      "secret already exists",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "config-key",
			value:     "test-value",
			mockFn: func(m *MockGSMClient) {
				m.On("CreateSecret", mock.Anything, mock.MatchedBy(func(req *secretmanagerpb.CreateSecretRequest) bool {
					expectedReq := &secretmanagerpb.CreateSecretRequest{
						Parent:   "projects/test-project",
						SecretId: "test-prefix_dev_usw2_app_service_config-key",
						Secret: &secretmanagerpb.Secret{
							Replication: &secretmanagerpb.Replication{
								Replication: &secretmanagerpb.Replication_Automatic_{
									Automatic: &secretmanagerpb.Replication_Automatic{},
								},
							},
						},
					}
					return req.Parent == expectedReq.Parent &&
						req.SecretId == "test-prefix_dev_usw2_app_service_config-key" &&
						req.Secret.GetReplication().GetAutomatic() != nil
				})).Return(nil, errors.New("secret already exists"))

				m.On("AddSecretVersion", mock.Anything, mock.MatchedBy(func(req *secretmanagerpb.AddSecretVersionRequest) bool {
					expectedReq := &secretmanagerpb.AddSecretVersionRequest{
						Parent: "projects/test-project/secrets/test-prefix_dev_usw2_app_service_config-key",
						Payload: &secretmanagerpb.SecretPayload{
							Data: []byte("test-value"),
						},
					}
					return req.Parent == expectedReq.Parent &&
						string(req.Payload.Data) == string(expectedReq.Payload.Data)
				})).Return(&secretmanagerpb.SecretVersion{}, nil)
			},
			wantErr: false,
		},
		{
			name:      "create secret error",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "config-key",
			value:     "test-value",
			mockFn: func(m *MockGSMClient) {
				m.On("CreateSecret", mock.Anything, mock.MatchedBy(func(req *secretmanagerpb.CreateSecretRequest) bool {
					expectedReq := &secretmanagerpb.CreateSecretRequest{
						Parent:   "projects/test-project",
						SecretId: "test-prefix_dev_usw2_app_service_config-key",
						Secret: &secretmanagerpb.Secret{
							Replication: &secretmanagerpb.Replication{
								Replication: &secretmanagerpb.Replication_Automatic_{
									Automatic: &secretmanagerpb.Replication_Automatic{},
								},
							},
						},
					}
					return req.Parent == expectedReq.Parent &&
						req.SecretId == "test-prefix_dev_usw2_app_service_config-key" &&
						req.Secret.GetReplication().GetAutomatic() != nil
				})).Return(nil, errors.New("internal error"))
			},
			wantErr: true,
		},
		{
			name:      "add version error",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "config-key",
			value:     "test-value",
			mockFn: func(m *MockGSMClient) {
				m.On("CreateSecret", mock.Anything, mock.MatchedBy(func(req *secretmanagerpb.CreateSecretRequest) bool {
					expectedReq := &secretmanagerpb.CreateSecretRequest{
						Parent:   "projects/test-project",
						SecretId: "test-prefix_dev_usw2_app_service_config-key",
						Secret: &secretmanagerpb.Secret{
							Replication: &secretmanagerpb.Replication{
								Replication: &secretmanagerpb.Replication_Automatic_{
									Automatic: &secretmanagerpb.Replication_Automatic{},
								},
							},
						},
					}
					return req.Parent == expectedReq.Parent &&
						req.SecretId == "test-prefix_dev_usw2_app_service_config-key" &&
						req.Secret.GetReplication().GetAutomatic() != nil
				})).Return(&secretmanagerpb.Secret{
					Name: "projects/test-project/secrets/test-prefix_dev_usw2_app_service_config-key",
				}, nil)

				m.On("AddSecretVersion", mock.Anything, mock.MatchedBy(func(req *secretmanagerpb.AddSecretVersionRequest) bool {
					expectedReq := &secretmanagerpb.AddSecretVersionRequest{
						Parent: "projects/test-project/secrets/test-prefix_dev_usw2_app_service_config-key",
						Payload: &secretmanagerpb.SecretPayload{
							Data: []byte("test-value"),
						},
					}
					return req.Parent == expectedReq.Parent &&
						string(req.Payload.Data) == string(expectedReq.Payload.Data)
				})).Return(nil, errors.New("internal error"))
			},
			wantErr: true,
		},
		{
			name:      "invalid value type",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "config-key",
			value:     123, // Not a string
			mockFn:    func(m *MockGSMClient) {},
			wantErr:   true,
		},
		{
			name:      "empty stack",
			stack:     "",
			component: "app/service",
			key:       "config-key",
			value:     "test-value",
			mockFn:    func(m *MockGSMClient) {},
			wantErr:   true,
		},
		{
			name:      "empty component",
			stack:     "dev-usw2",
			component: "",
			key:       "config-key",
			value:     "test-value",
			mockFn:    func(m *MockGSMClient) {},
			wantErr:   true,
		},
		{
			name:      "empty key",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "",
			value:     "test-value",
			mockFn:    func(m *MockGSMClient) {},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockGSMClient{}
			if tt.mockFn != nil {
				tt.mockFn(mockClient)
			}

			store := &GSMStore{
				client:         mockClient,
				projectID:      "test-project",
				prefix:         testPrefix,
				stackDelimiter: &testDelimiter,
			}

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

func TestGSMStore_Get(t *testing.T) {
	testPrefix := "test-prefix"
	testDelimiter := "-"

	tests := []struct {
		name      string
		stack     string
		component string
		key       string
		mockFn    func(*MockGSMClient)
		want      interface{}
		wantErr   bool
	}{
		{
			name:      "successful get",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "config-key",
			mockFn: func(m *MockGSMClient) {
				m.On("AccessSecretVersion", mock.Anything, mock.MatchedBy(func(req *secretmanagerpb.AccessSecretVersionRequest) bool {
					expectedReq := &secretmanagerpb.AccessSecretVersionRequest{
						Name: "projects/test-project/secrets/test-prefix_dev_usw2_app_service_config-key/versions/latest",
					}
					return req.Name == expectedReq.Name
				})).Return(&secretmanagerpb.AccessSecretVersionResponse{
					Payload: &secretmanagerpb.SecretPayload{
						Data: []byte("test-value"),
					},
				}, nil)
			},
			want:    "test-value",
			wantErr: false,
		},
		{
			name:      "secret not found",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "non-existent-key",
			mockFn: func(m *MockGSMClient) {
				m.On("AccessSecretVersion", mock.Anything, mock.MatchedBy(func(req *secretmanagerpb.AccessSecretVersionRequest) bool {
					expectedReq := &secretmanagerpb.AccessSecretVersionRequest{
						Name: "projects/test-project/secrets/test-prefix_dev_usw2_app_service_non-existent-key/versions/latest",
					}
					return req.Name == expectedReq.Name
				})).Return(nil, errors.New("secret not found"))
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:      "empty stack",
			stack:     "",
			component: "app/service",
			key:       "config-key",
			mockFn:    func(m *MockGSMClient) {},
			want:      nil,
			wantErr:   true,
		},
		{
			name:      "empty component",
			stack:     "dev-usw2",
			component: "",
			key:       "config-key",
			mockFn:    func(m *MockGSMClient) {},
			want:      nil,
			wantErr:   true,
		},
		{
			name:      "empty key",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "",
			mockFn:    func(m *MockGSMClient) {},
			want:      nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(MockGSMClient)
			tt.mockFn(mockClient)

			store := &GSMStore{
				client:         mockClient,
				prefix:         testPrefix,
				stackDelimiter: aws.String(testDelimiter),
				projectID:      "test-project",
			}
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

func TestNewGSMStore(t *testing.T) {
	tests := []struct {
		name        string
		options     GSMStoreOptions
		expectError bool
	}{
		{
			name: "valid configuration",
			options: GSMStoreOptions{
				ProjectID:      "test-project",
				Prefix:         aws.String("test-prefix"),
				StackDelimiter: aws.String("-"),
			},
			expectError: false,
		},
		{
			name: "missing project ID",
			options: GSMStoreOptions{
				Prefix:         aws.String("test-prefix"),
				StackDelimiter: aws.String("-"),
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := NewGSMStore(tt.options)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, store)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, store)
			}
		})
	}

	// Test with credentials from environment variable
	t.Run("with credentials from env", func(t *testing.T) {
		if credPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"); credPath != "" {
			options := GSMStoreOptions{
				ProjectID:      "test-project",
				Prefix:         aws.String("test-prefix"),
				StackDelimiter: aws.String("-"),
			}
			store, err := NewGSMStore(options)
			assert.NoError(t, err)
			assert.NotNil(t, store)
		} else {
			t.Skip("GOOGLE_APPLICATION_CREDENTIALS not set")
		}
	})
}
