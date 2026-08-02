package providers

import (
	"context"
	"errors"
	"fmt"
	"testing"

	secretmanagerpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	storepkg "github.com/cloudposse/atmos/pkg/store"
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

// CreateSecret mocks the GSM CreateSecret API call.
func (m *MockGSMClient) CreateSecret(ctx context.Context, req *secretmanagerpb.CreateSecretRequest, opts ...gax.CallOption) (*secretmanagerpb.Secret, error) {
	args := m.Called(mock.Anything, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*secretmanagerpb.Secret), args.Error(1)
}

// AddSecretVersion mocks the GSM AddSecretVersion API call.
func (m *MockGSMClient) AddSecretVersion(ctx context.Context, req *secretmanagerpb.AddSecretVersionRequest, opts ...gax.CallOption) (*secretmanagerpb.SecretVersion, error) {
	args := m.Called(mock.Anything, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*secretmanagerpb.SecretVersion), args.Error(1)
}

// AccessSecretVersion mocks the GSM AccessSecretVersion API call.
func (m *MockGSMClient) AccessSecretVersion(ctx context.Context, req *secretmanagerpb.AccessSecretVersionRequest, opts ...gax.CallOption) (*secretmanagerpb.AccessSecretVersionResponse, error) {
	args := m.Called(mock.Anything, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*secretmanagerpb.AccessSecretVersionResponse), args.Error(1)
}

// GetSecretVersion mocks the GSM GetSecretVersion API call, which returns version metadata
// without accessing or decrypting the secret payload.
func (m *MockGSMClient) GetSecretVersion(ctx context.Context, req *secretmanagerpb.GetSecretVersionRequest, opts ...gax.CallOption) (*secretmanagerpb.SecretVersion, error) {
	args := m.Called(mock.Anything, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*secretmanagerpb.SecretVersion), args.Error(1)
}

// DeleteSecret mocks the GSM DeleteSecret API call.
func (m *MockGSMClient) DeleteSecret(ctx context.Context, req *secretmanagerpb.DeleteSecretRequest, opts ...gax.CallOption) error {
	args := m.Called(mock.Anything, req)
	return args.Error(0)
}

// Close mocks the GSM client Close method.
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

	// Mark initOnce as done so ensureClient() won't try to create a real client.
	store.initOnce.Do(func() {})

	return store
}

// gsmClientSecretCreationMock returns a setup function that configures mock expectations for secret creation.
func gsmClientSecretCreationMock(projectID string, secretId string, secretPayload string, replication *secretmanagerpb.Replication, err error) func(m *MockGSMClient) {
	parent := fmt.Sprintf("projects/%s", projectID)
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

func TestGSMStore_createSecret(t *testing.T) {
	tests := []struct {
		name       string
		returnErr  error
		expectErr  error  // sentinel matched with errors.Is; nil means success expected.
		expectName string // expected secret name on success.
	}{
		{
			name:       "already exists returns existing secret",
			returnErr:  status.Error(codes.AlreadyExists, "exists"),
			expectName: "projects/test-project/secrets/my-secret",
		},
		{
			name:      "not found",
			returnErr: status.Error(codes.NotFound, "missing"),
			expectErr: storepkg.ErrResourceNotFound,
		},
		{
			name:      "permission denied",
			returnErr: status.Error(codes.PermissionDenied, "denied"),
			expectErr: storepkg.ErrPermissionDenied,
		},
		{
			name:      "generic error",
			returnErr: fmt.Errorf("boom"),
			expectErr: storepkg.ErrCreateSecret,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockGSMClient{}
			store := newGSMStoreWithClient(mockClient, GSMStoreOptions{ProjectID: "test-project"})

			mockClient.On("CreateSecret", mock.Anything, mock.Anything).Return(nil, tt.returnErr)

			secret, err := store.createSecret(context.Background(), "my-secret")

			if tt.expectErr != nil {
				assert.ErrorIs(t, err, tt.expectErr)
				assert.Nil(t, secret)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectName, secret.GetName())
			}
			mockClient.AssertExpectations(t)
		})
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
			// A stack-scoped secret coordinate omits the component segment.
			name:      "stack scoped set omits component",
			stack:     "dev-usw2",
			component: "",
			key:       "config-key",
			value:     "test-value",
			mockFn:    gsmClientSecretCreationMock("test-project", "test-prefix_dev_usw2_config-key", `"test-value"`, nil, nil),
		},
		{
			// A global secret coordinate omits both the stack and component segments.
			name:      "global scoped set omits stack and component",
			stack:     "",
			component: "",
			key:       "config-key",
			value:     "test-value",
			mockFn:    gsmClientSecretCreationMock("test-project", "test-prefix_config-key", `"test-value"`, nil, nil),
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
			// A stack-scoped secret coordinate omits the component segment.
			name:      "stack scoped get omits component",
			stack:     "dev-usw2",
			component: "",
			key:       "config-key",
			mockFn: func(m *MockGSMClient) {
				m.On("AccessSecretVersion", mock.Anything, mock.MatchedBy(func(req *secretmanagerpb.AccessSecretVersionRequest) bool {
					return req.Name == "projects/test-project/secrets/test-prefix_dev_usw2_config-key/versions/latest"
				})).Return(&secretmanagerpb.AccessSecretVersionResponse{
					Payload: &secretmanagerpb.SecretPayload{Data: []byte(`"test-value"`)},
				}, nil)
			},
			want: "test-value",
		},
		{
			// A global secret coordinate omits both the stack and component segments.
			name:      "global scoped get omits stack and component",
			stack:     "",
			component: "",
			key:       "config-key",
			mockFn: func(m *MockGSMClient) {
				m.On("AccessSecretVersion", mock.Anything, mock.MatchedBy(func(req *secretmanagerpb.AccessSecretVersionRequest) bool {
					return req.Name == "projects/test-project/secrets/test-prefix_config-key/versions/latest"
				})).Return(&secretmanagerpb.AccessSecretVersionResponse{
					Payload: &secretmanagerpb.SecretPayload{Data: []byte(`"test-value"`)},
				}, nil)
			},
			want: "test-value",
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

func TestGSMStore_Delete(t *testing.T) {
	testPrefix := "test-prefix"
	testDelimiter := "-"
	const secretName = "projects/test-project/secrets/test-prefix_dev_usw2_app_service_config-key"

	matchDelete := func(m *MockGSMClient, err error) {
		m.On("DeleteSecret", mock.Anything, mock.MatchedBy(func(req *secretmanagerpb.DeleteSecretRequest) bool {
			return req.Name == secretName
		})).Return(err)
	}

	tests := []struct {
		name      string
		stack     string
		component string
		key       string
		mockFn    func(*MockGSMClient)
		wantErr   error
	}{
		{
			name:      "success",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "config-key",
			mockFn:    func(m *MockGSMClient) { matchDelete(m, nil) },
		},
		{
			name:      "not found",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "config-key",
			mockFn:    func(m *MockGSMClient) { matchDelete(m, status.Error(codes.NotFound, "resource not found")) },
			wantErr:   storepkg.ErrResourceNotFound,
		},
		{
			name:      "permission denied",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "config-key",
			mockFn:    func(m *MockGSMClient) { matchDelete(m, status.Error(codes.PermissionDenied, "permission denied")) },
			wantErr:   storepkg.ErrPermissionDenied,
		},
		{
			name:      "generic error wrapped as delete failure",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "config-key",
			mockFn:    func(m *MockGSMClient) { matchDelete(m, ErrInternalError) },
			wantErr:   storepkg.ErrDeleteSecret,
		},
		{
			// A stack-scoped secret coordinate omits the component segment.
			name:      "stack scoped delete omits component",
			stack:     "dev-usw2",
			component: "",
			key:       "config-key",
			mockFn: func(m *MockGSMClient) {
				m.On("DeleteSecret", mock.Anything, mock.MatchedBy(func(req *secretmanagerpb.DeleteSecretRequest) bool {
					return req.Name == "projects/test-project/secrets/test-prefix_dev_usw2_config-key"
				})).Return(nil)
			},
		},
		{
			// A global secret coordinate omits both the stack and component segments.
			name:      "global scoped delete omits stack and component",
			stack:     "",
			component: "",
			key:       "config-key",
			mockFn: func(m *MockGSMClient) {
				m.On("DeleteSecret", mock.Anything, mock.MatchedBy(func(req *secretmanagerpb.DeleteSecretRequest) bool {
					return req.Name == "projects/test-project/secrets/test-prefix_config-key"
				})).Return(nil)
			},
		},
		{
			name:      "empty key",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "",
			mockFn:    func(m *MockGSMClient) {},
			wantErr:   storepkg.ErrEmptyKey,
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

			err := store.Delete(tt.stack, tt.component, tt.key)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
			mockClient.AssertExpectations(t)
		})
	}
}

func TestGSMStore_Has(t *testing.T) {
	testPrefix := "test-prefix"
	testDelimiter := "-"
	// Has builds the same version resource name that Get uses for AccessSecretVersion.
	const versionName = "projects/test-project/secrets/test-prefix_dev_usw2_app_service_config-key/versions/latest"

	matchGetVersion := func(m *MockGSMClient, version *secretmanagerpb.SecretVersion, err error) {
		call := m.On("GetSecretVersion", mock.Anything, mock.MatchedBy(func(req *secretmanagerpb.GetSecretVersionRequest) bool {
			return req.Name == versionName
		}))
		if err != nil {
			call.Return(nil, err)
		} else {
			call.Return(version, nil)
		}
	}

	tests := []struct {
		name    string
		mockFn  func(*MockGSMClient)
		want    bool
		wantErr error
	}{
		{
			name: "present",
			mockFn: func(m *MockGSMClient) {
				matchGetVersion(m, &secretmanagerpb.SecretVersion{Name: versionName}, nil)
			},
			want: true,
		},
		{
			name:   "absent",
			mockFn: func(m *MockGSMClient) { matchGetVersion(m, nil, status.Error(codes.NotFound, "resource not found")) },
			want:   false,
		},
		{
			name: "other error propagated",
			mockFn: func(m *MockGSMClient) {
				matchGetVersion(m, nil, status.Error(codes.PermissionDenied, "permission denied"))
			},
			want:    false,
			wantErr: storepkg.ErrPermissionDenied,
		},
		{
			// A non-gRPC-status error has no recognizable code, so it is wrapped as a generic
			// access failure rather than mapped to absence.
			name:    "non_status_error_wrapped",
			mockFn:  func(m *MockGSMClient) { matchGetVersion(m, nil, errors.New("boom")) },
			want:    false,
			wantErr: storepkg.ErrAccessSecret,
		},
		{
			// An empty key is rejected before any client call.
			name:    "empty_key",
			mockFn:  func(m *MockGSMClient) {},
			want:    false,
			wantErr: storepkg.ErrEmptyKey,
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

			key := "config-key"
			if tt.name == "empty_key" {
				key = ""
			}
			got, err := store.Has("dev-usw2", "app/service", key)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
			// Existence must be checked WITHOUT accessing/decrypting the payload.
			mockClient.AssertNotCalled(t, "AccessSecretVersion")
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
			name: "valid_options_with_credentials",
			options: GSMStoreOptions{
				ProjectID:      "test-project",
				Prefix:         stringPtr("test-prefix"),
				StackDelimiter: stringPtr("-"),
				Credentials:    stringPtr(`{"type": "service_account"}`),
			},
			// Client creation is deferred — no error at construction time.
			expectError: false,
		},
		{
			name: "missing project ID",
			options: GSMStoreOptions{
				Prefix:         stringPtr("test-prefix"),
				StackDelimiter: stringPtr("-"),
			},
			expectError: true,
		},
		{
			name: "valid_options_no_credentials",
			options: GSMStoreOptions{
				ProjectID:      "test-project",
				Prefix:         stringPtr("test-prefix"),
				StackDelimiter: stringPtr("-"),
			},
			// Client creation is deferred — succeeds at construction time.
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := NewGSMStore(tt.options, "")
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "project_id is required")
				assert.Nil(t, store)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, store)
			}
		})
	}
}

func TestNewGSMStore_LazyClientCreation(t *testing.T) {
	// Verify that NewGSMStore does not eagerly create a GCP client.
	// This is important because config loading creates stores before auth
	// credentials (e.g. GOOGLE_OAUTH_ACCESS_TOKEN) are established.
	store, err := NewGSMStore(GSMStoreOptions{
		ProjectID:      "test-project",
		Prefix:         stringPtr("test-prefix"),
		StackDelimiter: stringPtr("-"),
	}, "")
	assert.NoError(t, err)
	assert.NotNil(t, store)

	// The underlying client field should be nil (deferred).
	gsmStore := store.(*GSMStore)
	assert.Nil(t, gsmStore.client, "client should not be eagerly initialized")
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
			stackDelimiter: stringPtr("-"),
			stack:          "dev-usw2",
			component:      "app",
			key:            "config",
			expected:       "test-prefix_dev_usw2_app_config",
			wantErr:        false,
		},
		{
			name:           "path with slashes",
			prefix:         "test-prefix",
			stackDelimiter: stringPtr("-"),
			stack:          "dev-usw2",
			component:      "app/service",
			key:            "config/key",
			expected:       "test-prefix_dev_usw2_app_service_config_key",
			wantErr:        false,
		},
		{
			name:           "path with multiple delimiters",
			prefix:         "test/prefix",
			stackDelimiter: stringPtr("-"),
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
			stackDelimiter: stringPtr("_"),
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
					assert.ErrorIs(t, err, storepkg.ErrStackDelimiterNotSet)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			mockClient := new(MockGSMClient)
			store := &GSMStore{
				client:    mockClient,
				projectID: "test-project",
			}
			// Mark initOnce as done so ensureClient() won't try to create a real client.
			store.initOnce.Do(func() {})

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

func TestNewGSMStore_EmptyProjectID(t *testing.T) {
	_, err := NewGSMStore(GSMStoreOptions{}, "")
	assert.ErrorIs(t, err, storepkg.ErrProjectIDRequired)
}

func TestGSMStore_addSecretVersion_Errors(t *testing.T) {
	tests := []struct {
		name string
		code codes.Code
		want error
	}{
		{"not found", codes.NotFound, storepkg.ErrResourceNotFound},
		{"permission denied", codes.PermissionDenied, storepkg.ErrPermissionDenied},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := new(MockGSMClient)
			mc.On("CreateSecret", mock.Anything, mock.Anything).
				Return(&secretmanagerpb.Secret{Name: "projects/p/secrets/s"}, nil)
			mc.On("AddSecretVersion", mock.Anything, mock.Anything).
				Return(nil, status.Error(tt.code, "boom"))

			s := newGSMStoreWithClient(mc, GSMStoreOptions{ProjectID: "p"})
			err := s.Set("dev", "app", "k", "v")
			assert.ErrorIs(t, err, tt.want)
			mc.AssertExpectations(t)
		})
	}
}

func TestGSMStore_Set_MoreErrors(t *testing.T) {
	t.Run("nil value", func(t *testing.T) {
		mc := new(MockGSMClient)
		s := newGSMStoreWithClient(mc, GSMStoreOptions{ProjectID: "p"})
		assert.ErrorIs(t, s.Set("dev", "app", "k", nil), storepkg.ErrNilValue)
	})

	t.Run("marshal error", func(t *testing.T) {
		mc := new(MockGSMClient)
		s := newGSMStoreWithClient(mc, GSMStoreOptions{ProjectID: "p"})
		assert.ErrorIs(t, s.Set("dev", "app", "k", make(chan int)), storepkg.ErrSerializeJSON)
	})

	t.Run("createSecret error", func(t *testing.T) {
		mc := new(MockGSMClient)
		mc.On("CreateSecret", mock.Anything, mock.Anything).
			Return(nil, status.Error(codes.PermissionDenied, "boom"))
		s := newGSMStoreWithClient(mc, GSMStoreOptions{ProjectID: "p"})
		assert.ErrorIs(t, s.Set("dev", "app", "k", "v"), storepkg.ErrPermissionDenied)
		mc.AssertExpectations(t)
	})
}

func TestGSMStore_Get_GenericError(t *testing.T) {
	mc := new(MockGSMClient)
	mc.On("AccessSecretVersion", mock.Anything, mock.Anything).Return(nil, errors.New("boom"))
	s := newGSMStoreWithClient(mc, GSMStoreOptions{ProjectID: "p"})

	_, err := s.Get("dev", "app", "k")
	assert.ErrorIs(t, err, storepkg.ErrAccessSecret)
	mc.AssertExpectations(t)
}

func TestGSMStore_GetKey_MoreCases(t *testing.T) {
	t.Run("empty key", func(t *testing.T) {
		mc := new(MockGSMClient)
		s := newGSMStoreWithClient(mc, GSMStoreOptions{ProjectID: "p"})
		_, err := s.GetKey("")
		assert.ErrorIs(t, err, storepkg.ErrEmptyKey)
	})

	t.Run("not found", func(t *testing.T) {
		mc := new(MockGSMClient)
		mc.On("AccessSecretVersion", mock.Anything, mock.Anything).
			Return(nil, status.Error(codes.NotFound, "nope"))
		s := newGSMStoreWithClient(mc, GSMStoreOptions{ProjectID: "p"})
		_, err := s.GetKey("k")
		assert.ErrorIs(t, err, storepkg.ErrResourceNotFound)
		mc.AssertExpectations(t)
	})

	t.Run("prefix prepended and nil payload returns empty string", func(t *testing.T) {
		mc := new(MockGSMClient)
		mc.On("AccessSecretVersion", mock.Anything, mock.MatchedBy(func(req *secretmanagerpb.AccessSecretVersionRequest) bool {
			return req.Name == "projects/p/secrets/myprefix_k/versions/latest"
		})).Return(&secretmanagerpb.AccessSecretVersionResponse{}, nil) // Payload is nil.
		s := newGSMStoreWithClient(mc, GSMStoreOptions{ProjectID: "p", Prefix: stringPtr("myprefix")})
		v, err := s.GetKey("k")
		assert.NoError(t, err)
		assert.Equal(t, "", v)
		mc.AssertExpectations(t)
	})
}

func TestBuildGSMStore_ParseError(t *testing.T) {
	_, err := buildGSMStore("n", storepkg.StoreConfig{
		Options: map[string]interface{}{"project_id": []string{"x"}},
	})
	assert.ErrorIs(t, err, storepkg.ErrParseGSMOptions)
}
