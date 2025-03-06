package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	secretmanagerpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/googleapis/gax-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Define test error constants.
var (
	ErrInternalError = errors.New("internal error")
)

// MockGSMClient is a mock implementation of GSMClient.
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

// newGSMStoreWithClient creates a new GSMStore with a provided client (test helper).
func newGSMStoreWithClient(client GSMClient, options GSMStoreOptions) *GSMStore {
	store := &GSMStore{
		client:    client,
		projectID: options.ProjectID,
	}

	if options.Prefix != nil {
		store.prefix = *options.Prefix
	}

	if options.StackDelimiter != nil {
		store.stackDelimiter = options.StackDelimiter
	} else {
		defaultDelimiter := "-"
		store.stackDelimiter = &defaultDelimiter
	}

	return store
}

func TestGSMStore_Set(t *testing.T) {
	testPrefix := "test-prefix"
	testDelimiter := "-"

	tests := []struct {
		name      string
		stack     string
		component string
		key       string
		value     any
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
						req.SecretId == expectedReq.SecretId &&
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
						req.SecretId == expectedReq.SecretId &&
						req.Secret.GetReplication().GetAutomatic() != nil
				})).Return(nil, status.Error(codes.AlreadyExists, "secret already exists"))

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
						req.SecretId == expectedReq.SecretId &&
						req.Secret.GetReplication().GetAutomatic() != nil
				})).Return(nil, fmt.Errorf("internal error: %w", ErrInternalError))
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
						req.SecretId == expectedReq.SecretId &&
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
				})).Return(nil, fmt.Errorf("internal error: %w", ErrInternalError))
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

			store := newGSMStoreWithClient(mockClient, GSMStoreOptions{
				ProjectID:      "test-project",
				Prefix:         &testPrefix,
				StackDelimiter: &testDelimiter,
			})

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
		want      any
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
			key:       "config-key",
			mockFn: func(m *MockGSMClient) {
				m.On("AccessSecretVersion", mock.Anything, mock.MatchedBy(func(req *secretmanagerpb.AccessSecretVersionRequest) bool {
					expectedReq := &secretmanagerpb.AccessSecretVersionRequest{
						Name: "projects/test-project/secrets/test-prefix_dev_usw2_app_service_config-key/versions/latest",
					}
					return req.Name == expectedReq.Name
				})).Return(nil, status.Error(codes.NotFound, "resource not found"))
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:      "permission denied",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "config-key",
			mockFn: func(m *MockGSMClient) {
				m.On("AccessSecretVersion", mock.Anything, mock.MatchedBy(func(req *secretmanagerpb.AccessSecretVersionRequest) bool {
					expectedReq := &secretmanagerpb.AccessSecretVersionRequest{
						Name: "projects/test-project/secrets/test-prefix_dev_usw2_app_service_config-key/versions/latest",
					}
					return req.Name == expectedReq.Name
				})).Return(nil, status.Error(codes.PermissionDenied, "permission denied for secret"))
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

			store := newGSMStoreWithClient(mockClient, GSMStoreOptions{
				ProjectID:      "test-project",
				Prefix:         &testPrefix,
				StackDelimiter: &testDelimiter,
			})

			got, err := store.Get(tt.stack, tt.component, tt.key)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.name == "permission denied" {
					assert.Contains(t, err.Error(), "permission denied for secret")
				} else if tt.name == "secret not found" {
					assert.Contains(t, err.Error(), "resource not found")
				}
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
		skipMessage string
	}{
		{
			name: "valid_options_invalid_credentials",
			options: GSMStoreOptions{
				ProjectID:      "test-project",
				Prefix:         aws.String("test-prefix"),
				StackDelimiter: aws.String("-"),
				Credentials:    aws.String(`{"type": "service_account"}`), // Add minimal credentials
			},
			expectError: true, // Error is expected because the minimal credentials are incomplete for authentication
		},
		{
			name: "missing project ID",
			options: GSMStoreOptions{
				Prefix:         aws.String("test-prefix"),
				StackDelimiter: aws.String("-"),
			},
			expectError: true,
		},
		{
			name: "with credentials from env",
			options: GSMStoreOptions{
				ProjectID:      "test-project",
				Prefix:         aws.String("test-prefix"),
				StackDelimiter: aws.String("-"),
			},
			expectError: false,
			skipMessage: "GOOGLE_APPLICATION_CREDENTIALS environment variable not set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipMessage != "" && os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") == "" {
				t.Skip(tt.skipMessage)
			}
			store, err := NewGSMStore(tt.options)
			if tt.expectError {
				assert.Error(t, err)
				if tt.name == "missing project ID" {
					assert.Contains(t, err.Error(), "project_id is required")
				} else if tt.name == "valid_options_invalid_credentials" {
					assert.Contains(t, err.Error(), "failed to create Secret Manager client")
				} else {
					assert.Contains(t, err.Error(), "failed to create Secret Manager client")
				}
				assert.Nil(t, store)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, store)
			}
		})
	}
}

func TestGSMStore_GetKey(t *testing.T) {
	tests := []struct {
		name           string
		prefix         string
		stackDelimiter *string
		stack          string
		component      string
		key            string
		expected       string
		wantErr        bool
	}{
		{
			name:           "basic path",
			prefix:         "test-prefix",
			stackDelimiter: aws.String("-"),
			stack:          "dev-usw2",
			component:      "app",
			key:            "config",
			expected:       "test-prefix_dev_usw2_app_config",
			wantErr:        false,
		},
		{
			name:           "path with slashes",
			prefix:         "test-prefix",
			stackDelimiter: aws.String("-"),
			stack:          "dev-usw2",
			component:      "app/service",
			key:            "config/key",
			expected:       "test-prefix_dev_usw2_app_service_config_key",
			wantErr:        false,
		},
		{
			name:           "path with multiple delimiters",
			prefix:         "test/prefix",
			stackDelimiter: aws.String("-"),
			stack:          "dev-usw2-prod",
			component:      "app/service/db",
			key:            "config-key-name",
			expected:       "test_prefix_dev_usw2_prod_app_service_db_config-key-name",
			wantErr:        false,
		},
		{
			name:           "empty stack delimiter",
			prefix:         "prefix",
			stackDelimiter: nil,
			stack:          "dev",
			component:      "app",
			key:            "key",
			wantErr:        true,
		},
		{
			name:           "overridden stack delimiter",
			prefix:         "test-prefix",
			stackDelimiter: aws.String("_"),
			stack:          "dev_usw2_prod",
			component:      "app/service",
			key:            "config-key",
			expected:       "test-prefix_dev_usw2_prod_app_service_config-key",
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &GSMStore{
				prefix:         tt.prefix,
				stackDelimiter: tt.stackDelimiter,
			}

			got, err := store.getKey(tt.stack, tt.component, tt.key)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.name == "empty stack delimiter" {
					assert.ErrorIs(t, err, ErrStackDelimiterNotSet)
				}
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}
