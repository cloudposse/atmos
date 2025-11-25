package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	secretmanagerpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/gax-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Define test error constants.
var (
	ErrInternalError  = errors.New("internal error")
	ErrTransientError = errors.New("transient error")
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

	store.replication = createReplicationFromLocations(options.Locations)

	return store
}

func gsmClientSecretCreationMock(projectID string, secretId string, secretPayload string, replication *secretmanagerpb.Replication, err error) func(m *MockGSMClient) {
	parent := fmt.Sprintf("projects/%s", "test-project")
	return func(m *MockGSMClient) {
		if replication == nil {
			replication = &secretmanagerpb.Replication{
				Replication: &secretmanagerpb.Replication_Automatic_{
					Automatic: &secretmanagerpb.Replication_Automatic{},
				},
			}
		}

		m.On("CreateSecret", mock.Anything, mock.MatchedBy(func(req *secretmanagerpb.CreateSecretRequest) bool {
			expectedReq := &secretmanagerpb.CreateSecretRequest{
				Parent:   parent,
				SecretId: secretId,
				Secret: &secretmanagerpb.Secret{
					Replication: replication,
				},
			}

			replicationMatched := false
			if replication.GetAutomatic() != nil {
				replicationMatched = req.Secret.GetReplication().GetAutomatic() != nil
			} else {
				// Ensure both replication configurations support user-managed replication
				if replication.GetUserManaged() == nil || req.Secret.GetReplication().GetUserManaged() == nil {
					return false
				}
				var expectedLocations []string
				var receivedLocations []string
				for _, replica := range replication.GetUserManaged().Replicas {
					expectedLocations = append(expectedLocations, replica.Location)
				}
				for _, replica := range req.Secret.GetReplication().GetUserManaged().Replicas {
					receivedLocations = append(receivedLocations, replica.Location)
				}
				replicationMatched = cmp.Diff(expectedLocations, receivedLocations) == ""
			}

			return req.Parent == expectedReq.Parent &&
				req.SecretId == expectedReq.SecretId &&
				replicationMatched
		})).Return(&secretmanagerpb.Secret{
			Name: fmt.Sprintf("%s/secrets/%s", parent, secretId),
		}, nil)

		var ret *secretmanagerpb.SecretVersion
		if err == nil {
			ret = &secretmanagerpb.SecretVersion{}
		}

		m.On("AddSecretVersion", mock.Anything, mock.MatchedBy(func(req *secretmanagerpb.AddSecretVersionRequest) bool {
			expectedReq := &secretmanagerpb.AddSecretVersionRequest{
				Parent: fmt.Sprintf("%s/secrets/%s", parent, secretId),
				Payload: &secretmanagerpb.SecretPayload{
					Data: []byte(secretPayload),
				},
			}
			return req.Parent == expectedReq.Parent &&
				string(req.Payload.Data) == string(expectedReq.Payload.Data)
		})).Return(ret, err)
	}
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
		locations []string
		mockFn    func(*MockGSMClient)
		wantErr   bool
	}{
		{
			name:      "successful set",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "config-key",
			value:     "test-value",
			mockFn:    gsmClientSecretCreationMock("test-project", "test-prefix_dev_usw2_app_service_config-key", `"test-value"`, nil, nil),
			wantErr:   false,
		},
		{
			name:      "secret already exists",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "config-key",
			value:     "test-value",
			mockFn:    gsmClientSecretCreationMock("test-project", "test-prefix_dev_usw2_app_service_config-key", `"test-value"`, nil, nil),
			wantErr:   false,
		},
		{
			name:      "create secret error",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "config-key",
			value:     "test-value",
			mockFn:    gsmClientSecretCreationMock("test-project", "test-prefix_dev_usw2_app_service_config-key", `"test-value"`, nil, fmt.Errorf("internal error: %w", ErrInternalError)),
			wantErr:   true,
		},

		{
			name:      "create secret - transient error",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "config-key",
			value:     "test-value",
			mockFn:    gsmClientSecretCreationMock("test-project", "test-prefix_dev_usw2_app_service_config-key", `"test-value"`, nil, fmt.Errorf("transient error: %w", ErrTransientError)),
			wantErr:   true,
		},
		{
			name:      "add version error",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "config-key",
			value:     "test-value",
			mockFn:    gsmClientSecretCreationMock("test-project", "test-prefix_dev_usw2_app_service_config-key", `"test-value"`, nil, fmt.Errorf("internal error: %w", ErrInternalError)),
			wantErr:   true,
		},
		{
			name:      "successful set with int",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "config-key",
			value:     123,
			mockFn:    gsmClientSecretCreationMock("test-project", "test-prefix_dev_usw2_app_service_config-key", `123`, nil, nil),
			wantErr:   false,
		},
		{
			name:      "successful_set_with_slice",
			stack:     "dev_usw2",
			component: "app/service",
			key:       "slice-key",
			value:     []string{"value1", "value2", "value3"},
			mockFn:    gsmClientSecretCreationMock("test-project", "test-prefix_dev_usw2_app_service_slice-key", `["value1","value2","value3"]`, nil, nil),
		},
		{
			name:      "successful_set_with_map",
			stack:     "dev_usw2",
			component: "app/service",
			key:       "map-key",
			value:     map[string]interface{}{"key1": "value1", "key2": 42, "key3": true},

			mockFn: gsmClientSecretCreationMock("test-project", "test-prefix_dev_usw2_app_service_map-key", `{"key1":"value1","key2":42,"key3":true}`, nil, nil),
		},
		{
			name:      "successful_set_automatic_replication",
			stack:     "dev_usw2",
			component: "app/service",
			key:       "map-key",
			value:     map[string]interface{}{"key1": "value1", "key2": 42, "key3": true},
			locations: []string{},
			mockFn:    gsmClientSecretCreationMock("test-project", "test-prefix_dev_usw2_app_service_map-key", `{"key1":"value1","key2":42,"key3":true}`, nil, nil),
		},
		{
			name:      "successful_set_user_managed_replication",
			stack:     "dev_usw2",
			component: "app/service",
			key:       "map-key",
			value:     map[string]interface{}{"key1": "value1", "key2": 42, "key3": true},
			locations: []string{"us-west1", "us-central1"},
			mockFn: gsmClientSecretCreationMock("test-project", "test-prefix_dev_usw2_app_service_map-key", `{"key1":"value1","key2":42,"key3":true}`,
				&secretmanagerpb.Replication{
					Replication: &secretmanagerpb.Replication_UserManaged_{
						UserManaged: &secretmanagerpb.Replication_UserManaged{
							Replicas: []*secretmanagerpb.Replication_UserManaged_Replica{
								{Location: "us-west1"},
								{Location: "us-central1"},
							},
						},
					},
				}, nil),
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
				Locations:      &tt.locations,
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
						Data: []byte(`"test-value"`),
					},
				}, nil)
			},
			want:    "test-value",
			wantErr: false,
		},
		{
			name:      "successful get - legacy value without json",
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
						Data: []byte(`test-value`),
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
				switch tt.name {
				case "permission denied":
					assert.Contains(t, err.Error(), "permission denied for secret")
				case "secret not found":
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
				t.Skipf("%s", tt.skipMessage)
			}
			store, err := NewGSMStore(tt.options)
			if tt.expectError {
				assert.Error(t, err)
				switch tt.name {
				case "missing project ID":
					assert.Contains(t, err.Error(), "project_id is required")
				default:
					assert.Contains(t, err.Error(), "failed to create client")
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
				replication:    createReplicationFromLocations(nil),
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

func TestGSMStore_GetKeyDirect(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		mockPayload   []byte
		mockError     error
		expectedValue interface{}
		expectError   bool
		errorContains string
	}{
		{
			name:          "successful string retrieval",
			key:           "app-config",
			mockPayload:   []byte("production"),
			mockError:     nil,
			expectedValue: "production",
			expectError:   false,
		},
		{
			name:          "successful JSON object retrieval",
			key:           "database-config",
			mockPayload:   []byte(`{"host":"localhost","port":5432}`),
			mockError:     nil,
			expectedValue: map[string]interface{}{"host": "localhost", "port": float64(5432)},
			expectError:   false,
		},
		{
			name:          "successful JSON array retrieval",
			key:           "server-list",
			mockPayload:   []byte(`["server1","server2","server3"]`),
			mockError:     nil,
			expectedValue: []interface{}{"server1", "server2", "server3"},
			expectError:   false,
		},
		{
			name:          "secret not found",
			key:           "nonexistent",
			mockPayload:   nil,
			mockError:     status.Error(codes.NotFound, "secret not found"),
			expectedValue: nil,
			expectError:   true,
			errorContains: "resource not found",
		},
		{
			name:          "empty secret value",
			key:           "empty-secret",
			mockPayload:   []byte(""),
			mockError:     nil,
			expectedValue: "",
			expectError:   false,
		},
		{
			name:          "malformed JSON returns as string",
			key:           "invalid-json",
			mockPayload:   []byte(`{"invalid": json`),
			mockError:     nil,
			expectedValue: `{"invalid": json`,
			expectError:   false,
		},
		{
			name:          "permission denied error",
			key:           "restricted",
			mockPayload:   nil,
			mockError:     status.Error(codes.PermissionDenied, "permission denied"),
			expectedValue: nil,
			expectError:   true,
			errorContains: "permission denied",
		},
		{
			name:          "internal server error",
			key:           "server-error",
			mockPayload:   nil,
			mockError:     status.Error(codes.Internal, "internal server error"),
			expectedValue: nil,
			expectError:   true,
			errorContains: "failed to access secret",
		},
		{
			name:          "key used exactly without prefix",
			key:           "my-exact-secret-name",
			mockPayload:   []byte(`"secret-value"`),
			mockError:     nil,
			expectedValue: "secret-value",
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			mockClient := new(MockGSMClient)
			store := &GSMStore{
				client:    mockClient,
				projectID: "test-project",
			}

			// Set up mock expectations
			expectedFullPath := fmt.Sprintf("projects/%s/secrets/%s/versions/latest", "test-project", tt.key)
			if tt.mockError != nil {
				mockClient.On("AccessSecretVersion", mock.Anything, &secretmanagerpb.AccessSecretVersionRequest{
					Name: expectedFullPath,
				}).Return(nil, tt.mockError)
			} else {
				mockClient.On("AccessSecretVersion", mock.Anything, &secretmanagerpb.AccessSecretVersionRequest{
					Name: expectedFullPath,
				}).Return(&secretmanagerpb.AccessSecretVersionResponse{
					Payload: &secretmanagerpb.SecretPayload{
						Data: tt.mockPayload,
					},
				}, nil)
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
			mockClient.AssertExpectations(t)
		})
	}
}

func TestGSMStore_GetKey_WithPrefix(t *testing.T) {
	testPrefix := "test-prefix"

	t.Run("prefix_is_not_added_to_key", func(t *testing.T) {
		// Arrange
		mockClient := new(MockGSMClient)
		store := &GSMStore{
			client:    mockClient,
			projectID: "test-project",
			prefix:    testPrefix,
		}

		// The key should be used exactly as provided, WITHOUT the prefix prepended
		key := "my-secret"
		expectedFullPath := fmt.Sprintf("projects/%s/secrets/%s/versions/latest", "test-project", key)

		mockClient.On("AccessSecretVersion", mock.Anything, &secretmanagerpb.AccessSecretVersionRequest{
			Name: expectedFullPath,
		}).Return(&secretmanagerpb.AccessSecretVersionResponse{
			Payload: &secretmanagerpb.SecretPayload{
				Data: []byte(`"test-value"`),
			},
		}, nil)

		// Act
		result, err := store.GetKey(key)

		// Assert
		assert.NoError(t, err)
		assert.Equal(t, "test-value", result)
		mockClient.AssertExpectations(t)
	})
}
