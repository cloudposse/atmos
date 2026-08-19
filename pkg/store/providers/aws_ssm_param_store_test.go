package providers

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	ststypes "github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	storepkg "github.com/cloudposse/atmos/pkg/store"
	"github.com/cloudposse/atmos/tests"
)

func TestSSMStore_Set(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockSSM := NewMockSSMClient(ctrl)
	mockSTS := NewMockSTSClient(ctrl)
	mockAssumedSSM := NewMockSSMClient(ctrl)
	stackDelimiter := "/"
	awsConfig := &aws.Config{Region: "us-west-2"}

	store := &SSMStore{
		client:         mockSSM,
		prefix:         "/test-prefix",
		stackDelimiter: &stackDelimiter,
		awsConfig:      awsConfig,
		newSTSClient: func(cfg aws.Config) STSClient {
			return mockSTS
		},
		newSSMClient: func(cfg aws.Config) SSMClient {
			return mockAssumedSSM
		},
	}

	tests := []struct {
		name              string
		stack             string
		component         string
		key               string
		value             interface{}
		secret            bool
		writeRoleArn      *string
		nilStackDelimiter bool
		mockSetup         func(*MockSSMClient, *MockSSMClient, *MockSTSClient)
		wantErr           bool
	}{
		{
			name:      "successful_set",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "config-key",
			value:     "test-value",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().PutParameter(gomock.Any(), &ssm.PutParameterInput{
					Name:      aws.String("/test-prefix/dev/usw2/app/service/config-key"),
					Value:     aws.String(`"test-value"`),
					Type:      types.ParameterTypeString,
					Overwrite: aws.Bool(true),
				}).Return(&ssm.PutParameterOutput{}, nil)
			},
		},
		{
			name:      "secret_string_is_stored_raw",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "secret-key",
			value:     "test-value",
			secret:    true,
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().PutParameter(gomock.Any(), &ssm.PutParameterInput{
					Name:      aws.String("/test-prefix/dev/usw2/app/service/secret-key"),
					Value:     aws.String("test-value"),
					Type:      types.ParameterTypeSecureString,
					Overwrite: aws.Bool(true),
				}).Return(&ssm.PutParameterOutput{}, nil)
			},
		},
		{
			name:      "secret_structured_value_is_json_encoded",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "secret-config",
			value:     map[string]interface{}{"key": "value"},
			secret:    true,
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().PutParameter(gomock.Any(), &ssm.PutParameterInput{
					Name:      aws.String("/test-prefix/dev/usw2/app/service/secret-config"),
					Value:     aws.String(`{"key":"value"}`),
					Type:      types.ParameterTypeSecureString,
					Overwrite: aws.Bool(true),
				}).Return(&ssm.PutParameterOutput{}, nil)
			},
		},
		{
			// SSM PutParameter rejects zero-length values, so an empty secret string
			// must keep its JSON encoding (`""`) instead of being written raw.
			name:      "secret_empty_string_is_json_encoded",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "secret-key",
			value:     "",
			secret:    true,
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().PutParameter(gomock.Any(), &ssm.PutParameterInput{
					Name:      aws.String("/test-prefix/dev/usw2/app/service/secret-key"),
					Value:     aws.String(`""`),
					Type:      types.ParameterTypeSecureString,
					Overwrite: aws.Bool(true),
				}).Return(&ssm.PutParameterOutput{}, nil)
			},
		},
		{
			name:      "secret_non_serializable_value_returns_error",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "secret-config",
			value:     make(chan int),
			secret:    true,
			wantErr:   true,
		},
		{
			name:      "successful_set_with_slice",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "slice-key",
			value:     []string{"value1", "value2", "value3"},
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().PutParameter(gomock.Any(), &ssm.PutParameterInput{
					Name:      aws.String("/test-prefix/dev/usw2/app/service/slice-key"),
					Value:     aws.String(`["value1","value2","value3"]`),
					Type:      types.ParameterTypeString,
					Overwrite: aws.Bool(true),
				}).Return(&ssm.PutParameterOutput{}, nil)
			},
		},
		{
			name:      "successful_set_with_map",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "map-key",
			value:     map[string]interface{}{"key1": "value1", "key2": 42, "key3": true},
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().PutParameter(gomock.Any(), &ssm.PutParameterInput{
					Name:      aws.String("/test-prefix/dev/usw2/app/service/map-key"),
					Value:     aws.String(`{"key1":"value1","key2":42,"key3":true}`),
					Type:      types.ParameterTypeString,
					Overwrite: aws.Bool(true),
				}).Return(&ssm.PutParameterOutput{}, nil)
			},
		},
		{
			name:      "aws_error",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "config-key",
			value:     "test-value",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().PutParameter(gomock.Any(), gomock.Any()).Return(nil, errors.New("aws error"))
			},
			wantErr: true,
		},
		{
			name:      "invalid_value_type",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "config-key",
			value:     123,
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().PutParameter(gomock.Any(), gomock.Any()).Return(nil, errors.New("invalid value type"))
			},
			wantErr: true,
		},
		{
			// A stack-scoped secret coordinate omits the component segment.
			name:      "stack_scoped_set_omits_component",
			stack:     "dev/usw2/app",
			component: "",
			key:       "config-key",
			value:     "test-value",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().PutParameter(gomock.Any(), &ssm.PutParameterInput{
					Name:      aws.String("/test-prefix/dev/usw2/app/config-key"),
					Value:     aws.String(`"test-value"`),
					Type:      types.ParameterTypeString,
					Overwrite: aws.Bool(true),
				}).Return(&ssm.PutParameterOutput{}, nil)
			},
		},
		{
			// A global secret coordinate omits both the stack and component segments.
			name:      "global_scoped_set_omits_stack_and_component",
			stack:     "",
			component: "",
			key:       "config-key",
			value:     "test-value",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().PutParameter(gomock.Any(), &ssm.PutParameterInput{
					Name:      aws.String("/test-prefix/config-key"),
					Value:     aws.String(`"test-value"`),
					Type:      types.ParameterTypeString,
					Overwrite: aws.Bool(true),
				}).Return(&ssm.PutParameterOutput{}, nil)
			},
		},
		{
			name:      "empty_key",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "",
			value:     "test-value",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {},
			wantErr:   true,
		},
		{
			name:              "nil_stack_delimiter",
			stack:             "dev/usw2/app",
			component:         "service",
			key:               "config-key",
			value:             "test-value",
			nilStackDelimiter: true,
			mockSetup:         func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {},
			wantErr:           true,
		},
		{
			name:      "complex_stack_name_with_multiple_delimiters",
			stack:     "dev/usw2/prod/app",
			component: "service",
			key:       "config-key",
			value:     "test-value",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().PutParameter(gomock.Any(), &ssm.PutParameterInput{
					Name:      aws.String("/test-prefix/dev/usw2/prod/app/service/config-key"),
					Value:     aws.String(`"test-value"`),
					Type:      types.ParameterTypeString,
					Overwrite: aws.Bool(true),
				}).Return(&ssm.PutParameterOutput{}, nil)
			},
		},
		{
			name:         "successful_set_with_write_role",
			stack:        "dev/usw2/app",
			component:    "service",
			key:          "config-key",
			value:        "test-value",
			writeRoleArn: aws.String("arn:aws:iam::123456789012:role/write-role"),
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSTS.EXPECT().AssumeRole(gomock.Any(), &sts.AssumeRoleInput{
					RoleArn:         aws.String("arn:aws:iam::123456789012:role/write-role"),
					RoleSessionName: aws.String("atmos-ssm-session"),
				}).Return(&sts.AssumeRoleOutput{
					Credentials: &ststypes.Credentials{
						AccessKeyId:     aws.String("AKIATEST"),
						SecretAccessKey: aws.String("secret"),
						SessionToken:    aws.String("token"),
					},
				}, nil)

				mockAssumedSSM.EXPECT().PutParameter(gomock.Any(), &ssm.PutParameterInput{
					Name:      aws.String("/test-prefix/dev/usw2/app/service/config-key"),
					Value:     aws.String(`"test-value"`),
					Type:      types.ParameterTypeString,
					Overwrite: aws.Bool(true),
				}).Return(&ssm.PutParameterOutput{}, nil)
			},
		},
		{
			name:         "failed_role_assumption_for_write",
			stack:        "dev/usw2/app",
			component:    "service",
			key:          "config-key",
			value:        "test-value",
			writeRoleArn: aws.String("arn:aws:iam::123456789012:role/write-role"),
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSTS.EXPECT().AssumeRole(gomock.Any(), &sts.AssumeRoleInput{
					RoleArn:         aws.String("arn:aws:iam::123456789012:role/write-role"),
					RoleSessionName: aws.String("atmos-ssm-session"),
				}).Return(nil, fmt.Errorf("failed to assume role"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mockSetup != nil {
				tt.mockSetup(mockSSM, mockAssumedSSM, mockSTS)
			}

			store.writeRoleArn = tt.writeRoleArn
			store.secret = tt.secret
			if tt.nilStackDelimiter {
				store.stackDelimiter = nil
			} else {
				store.stackDelimiter = &stackDelimiter
			}
			err := store.Set(tt.stack, tt.component, tt.key, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("SSMStore.Set() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

// TestSSMStore_Has_NoDecryption proves the existence check never decrypts: GetParameter is
// called with WithDecryption=false (so kms:Decrypt is not required), a present parameter reports
// true, and a ParameterNotFound reports false.
func TestSSMStore_Has_NoDecryption(t *testing.T) {
	stackDelimiter := "/"
	param := "/test-prefix/dev/usw2/app/service/config-key"

	t.Run("present_without_decryption", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockSSM := NewMockSSMClient(ctrl)
		store := &SSMStore{client: mockSSM, prefix: "/test-prefix", stackDelimiter: &stackDelimiter}
		mockSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
			Name:           aws.String(param),
			WithDecryption: aws.Bool(false),
		}).Return(&ssm.GetParameterOutput{Parameter: &types.Parameter{Value: aws.String("ignored")}}, nil)

		ok, err := store.Has("dev/usw2/app", "service", "config-key")
		require.NoError(t, err)
		assert.True(t, ok) // fails if a WithDecryption=true call was made instead.
	})

	t.Run("absent_returns_false", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockSSM := NewMockSSMClient(ctrl)
		store := &SSMStore{client: mockSSM, prefix: "/test-prefix", stackDelimiter: &stackDelimiter}
		mockSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
			Name:           aws.String(param),
			WithDecryption: aws.Bool(false),
		}).Return((*ssm.GetParameterOutput)(nil), &types.ParameterNotFound{})

		ok, err := store.Has("dev/usw2/app", "service", "config-key")
		require.NoError(t, err)
		assert.False(t, ok) // fails if GetParameter stops being called.
	})
}

// TestSSMStore_Has_ReadRole proves Has assumes the configured read role and runs the
// (decryption-free) existence check against the assumed-role client.
func TestSSMStore_Has_ReadRole(t *testing.T) {
	stackDelimiter := "/"
	ctrl := gomock.NewController(t)
	mockSSM := NewMockSSMClient(ctrl)
	mockSTS := NewMockSTSClient(ctrl)
	mockAssumedSSM := NewMockSSMClient(ctrl)

	store := &SSMStore{
		client:         mockSSM,
		prefix:         "/test-prefix",
		stackDelimiter: &stackDelimiter,
		awsConfig:      &aws.Config{Region: "us-west-2"},
		readRoleArn:    aws.String("arn:aws:iam::123456789012:role/read-role"),
		newSTSClient:   func(cfg aws.Config) STSClient { return mockSTS },
		newSSMClient:   func(cfg aws.Config) SSMClient { return mockAssumedSSM },
	}

	mockSTS.EXPECT().AssumeRole(gomock.Any(), &sts.AssumeRoleInput{
		RoleArn:         aws.String("arn:aws:iam::123456789012:role/read-role"),
		RoleSessionName: aws.String("atmos-ssm-session"),
	}).Return(&sts.AssumeRoleOutput{
		Credentials: &ststypes.Credentials{
			AccessKeyId:     aws.String("AKIATEST"),
			SecretAccessKey: aws.String("secret"),
			SessionToken:    aws.String("token"),
		},
	}, nil)
	mockAssumedSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
		Name:           aws.String("/test-prefix/dev/usw2/app/service/config-key"),
		WithDecryption: aws.Bool(false),
	}).Return(&ssm.GetParameterOutput{Parameter: &types.Parameter{Value: aws.String("ignored")}}, nil) // existence check ran against the assumed-role client.

	ok, err := store.Has("dev/usw2/app", "service", "config-key")
	require.NoError(t, err)
	assert.True(t, ok)
}

// TestSSMStore_Has_ErrorPaths covers Has's non-success branches: empty key, getKey failure,
// read-role assumption failure, and a non-not-found GetParameter error.
func TestSSMStore_Has_ErrorPaths(t *testing.T) {
	stackDelimiter := "/"

	t.Run("empty_key", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		store := &SSMStore{client: NewMockSSMClient(ctrl), prefix: "/test-prefix", stackDelimiter: &stackDelimiter}
		_, err := store.Has("dev/usw2/app", "service", "")
		require.ErrorIs(t, err, storepkg.ErrEmptyKey)
	})

	t.Run("get_key_error_nil_delimiter", func(t *testing.T) {
		// A nil stack delimiter makes getKey fail before any client call.
		ctrl := gomock.NewController(t)
		store := &SSMStore{client: NewMockSSMClient(ctrl), prefix: "/test-prefix", stackDelimiter: nil}
		_, err := store.Has("dev/usw2/app", "service", "config-key")
		require.ErrorIs(t, err, storepkg.ErrGetKey)
	})

	t.Run("assume_read_role_fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockSTS := NewMockSTSClient(ctrl)
		store := &SSMStore{
			client:         NewMockSSMClient(ctrl),
			prefix:         "/test-prefix",
			stackDelimiter: &stackDelimiter,
			awsConfig:      &aws.Config{Region: "us-west-2"},
			readRoleArn:    aws.String("arn:aws:iam::123456789012:role/read-role"),
			newSTSClient:   func(cfg aws.Config) STSClient { return mockSTS },
			newSSMClient:   func(cfg aws.Config) SSMClient { return NewMockSSMClient(ctrl) },
		}
		mockSTS.EXPECT().AssumeRole(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("boom"))

		_, err := store.Has("dev/usw2/app", "service", "config-key")
		require.ErrorIs(t, err, storepkg.ErrAssumeRole)
	})

	t.Run("other_get_parameter_error_is_wrapped", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockSSM := NewMockSSMClient(ctrl)
		store := &SSMStore{client: mockSSM, prefix: "/test-prefix", stackDelimiter: &stackDelimiter}
		mockSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
			Name:           aws.String("/test-prefix/dev/usw2/app/service/config-key"),
			WithDecryption: aws.Bool(false),
		}).Return((*ssm.GetParameterOutput)(nil), errors.New("access denied"))

		_, err := store.Has("dev/usw2/app", "service", "config-key")
		require.ErrorIs(t, err, storepkg.ErrGetParameter)
	})
}

func TestSSMStore_Get(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockSSM := NewMockSSMClient(ctrl)
	mockSTS := NewMockSTSClient(ctrl)
	mockAssumedSSM := NewMockSSMClient(ctrl)
	stackDelimiter := "/"
	awsConfig := &aws.Config{Region: "us-west-2"}

	store := &SSMStore{
		client:         mockSSM,
		prefix:         "/test-prefix",
		stackDelimiter: &stackDelimiter,
		awsConfig:      awsConfig,
		newSTSClient: func(cfg aws.Config) STSClient {
			return mockSTS
		},
		newSSMClient: func(cfg aws.Config) SSMClient {
			return mockAssumedSSM
		},
	}

	tests := []struct {
		name        string
		stack       string
		component   string
		key         string
		secret      bool
		readRoleArn *string
		mockSetup   func(*MockSSMClient, *MockSSMClient, *MockSTSClient)
		want        interface{}
		wantErr     bool
	}{
		{
			name:      "successful_get",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "config-key",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/config-key"),
					WithDecryption: aws.Bool(true),
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`"test-value"`),
					},
				}, nil)
			},
			want: "test-value",
		},
		{
			name:      "successful_get_slice",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "slice-key",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/slice-key"),
					WithDecryption: aws.Bool(true),
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`["value1","value2","value3"]`),
					},
				}, nil)
			},
			want: []interface{}{"value1", "value2", "value3"},
		},
		{
			name:      "successful_get_map",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "map-key",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/map-key"),
					WithDecryption: aws.Bool(true),
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`{"key1":"value1","key2":42}`),
					},
				}, nil)
			},
			want: map[string]interface{}{"key1": "value1", "key2": float64(42)},
		},
		{
			name:      "aws_error",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "config-key",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), gomock.Any()).Return(nil, errors.New("aws error"))
			},
			wantErr: true,
		},
		{
			// A stack-scoped secret coordinate omits the component segment.
			name:      "stack_scoped_get_omits_component",
			stack:     "dev/usw2/app",
			component: "",
			key:       "config-key",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/config-key"),
					WithDecryption: aws.Bool(true),
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`"test-value"`),
					},
				}, nil)
			},
			want: "test-value",
		},
		{
			// A global secret coordinate omits both the stack and component segments.
			name:      "global_scoped_get_omits_stack_and_component",
			stack:     "",
			component: "",
			key:       "config-key",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/config-key"),
					WithDecryption: aws.Bool(true),
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`"test-value"`),
					},
				}, nil)
			},
			want: "test-value",
		},
		{
			name:      "empty_key",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {},
			wantErr:   true,
		},
		{
			name:      "parameter_not_found",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "non-existent-key",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), gomock.Any()).Return(nil, &types.ParameterNotFound{})
			},
			wantErr: true,
		},
		{
			name:      "non-json_value",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "plain-text-key",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/plain-text-key"),
					WithDecryption: aws.Bool(true),
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String("plain text value"),
					},
				}, nil)
			},
			want:    "plain text value",
			wantErr: false,
		},
		{
			name:      "malformed_json_value",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "malformed-json-key",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/malformed-json-key"),
					WithDecryption: aws.Bool(true),
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String("}invalid json{"),
					},
				}, nil)
			},
			want:    "}invalid json{",
			wantErr: false,
		},
		{
			name:      "integer_value",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "integer-key",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/integer-key"),
					WithDecryption: aws.Bool(true),
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`42`),
					},
				}, nil)
			},
			want: float64(42),
		},
		{
			name:      "float_value",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "float-key",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/float-key"),
					WithDecryption: aws.Bool(true),
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`3.14159`),
					},
				}, nil)
			},
			want: float64(3.14159),
		},
		{
			name:      "numeric_string",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "numeric-string-key",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/numeric-string-key"),
					WithDecryption: aws.Bool(true),
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`"42"`),
					},
				}, nil)
			},
			want: "42",
		},
		{
			// Secret stores write scalar strings raw (see Set), so a secret whose bytes
			// parse as a JSON number must round-trip byte-exact instead of being re-typed
			// to float64 (which would corrupt it to "100000" via %v formatting).
			name:      "secret_raw_scientific_notation_string_round_trips",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "secret-key",
			secret:    true,
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/secret-key"),
					WithDecryption: aws.Bool(true),
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`1e5`),
					},
				}, nil)
			},
			want: "1e5",
		},
		{
			// json.Unmarshal into any would lose precision on large integers (float64
			// mantissa); the raw bytes must survive.
			name:      "secret_raw_large_integer_round_trips",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "secret-key",
			secret:    true,
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/secret-key"),
					WithDecryption: aws.Bool(true),
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`12345678901234567890`),
					},
				}, nil)
			},
			want: "12345678901234567890",
		},
		{
			// A secret literally set to the string "null" must not come back as nil
			// (which %v formatting would render as "<nil>").
			name:      "secret_raw_null_string_round_trips",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "secret-key",
			secret:    true,
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/secret-key"),
					WithDecryption: aws.Bool(true),
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`null`),
					},
				}, nil)
			},
			want: "null",
		},
		{
			name:      "secret_raw_boolean_string_round_trips",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "secret-key",
			secret:    true,
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/secret-key"),
					WithDecryption: aws.Bool(true),
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`true`),
					},
				}, nil)
			},
			want: "true",
		},
		{
			// Empty secret strings are written JSON-encoded (`""`) because SSM rejects
			// zero-length values; they must decode back to an empty string.
			name:      "secret_empty_string_round_trips",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "secret-key",
			secret:    true,
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/secret-key"),
					WithDecryption: aws.Bool(true),
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`""`),
					},
				}, nil)
			},
			want: "",
		},
		{
			// Parameters written JSON-quoted by Atmos before raw scalar writes were
			// introduced must keep decoding, so existing stored secrets stay readable.
			name:      "secret_legacy_json_quoted_string_still_decodes",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "secret-key",
			secret:    true,
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/secret-key"),
					WithDecryption: aws.Bool(true),
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`"legacy-value"`),
					},
				}, nil)
			},
			want: "legacy-value",
		},
		{
			// Structured secrets are still JSON-encoded by Set and must decode so the
			// YQ query path receives a map.
			name:      "secret_structured_value_still_decodes",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "secret-key",
			secret:    true,
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/secret-key"),
					WithDecryption: aws.Bool(true),
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`{"key":"value"}`),
					},
				}, nil)
			},
			want: map[string]interface{}{"key": "value"},
		},
		{
			// Non-JSON secret strings (the common case after raw writes) come back verbatim.
			name:      "secret_raw_plain_string_round_trips",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "secret-key",
			secret:    true,
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/secret-key"),
					WithDecryption: aws.Bool(true),
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`hello world`),
					},
				}, nil)
			},
			want: "hello world",
		},
		{
			name:        "successful_get_with_read_role",
			stack:       "dev/usw2/app",
			component:   "service",
			key:         "config-key",
			readRoleArn: aws.String("arn:aws:iam::123456789012:role/read-role"),
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSTS.EXPECT().AssumeRole(gomock.Any(), &sts.AssumeRoleInput{
					RoleArn:         aws.String("arn:aws:iam::123456789012:role/read-role"),
					RoleSessionName: aws.String("atmos-ssm-session"),
				}).Return(&sts.AssumeRoleOutput{
					Credentials: &ststypes.Credentials{
						AccessKeyId:     aws.String("AKIATEST"),
						SecretAccessKey: aws.String("secret"),
						SessionToken:    aws.String("token"),
					},
				}, nil)

				mockAssumedSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/config-key"),
					WithDecryption: aws.Bool(true),
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`"test-value"`),
					},
				}, nil)
			},
			want: "test-value",
		},
		{
			name:        "failed_role_assumption_for_read",
			stack:       "dev/usw2/app",
			component:   "service",
			key:         "config-key",
			readRoleArn: aws.String("arn:aws:iam::123456789012:role/read-role"),
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSTS.EXPECT().AssumeRole(gomock.Any(), &sts.AssumeRoleInput{
					RoleArn:         aws.String("arn:aws:iam::123456789012:role/read-role"),
					RoleSessionName: aws.String("atmos-ssm-session"),
				}).Return(nil, fmt.Errorf("failed to assume role"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mockSetup != nil {
				tt.mockSetup(mockSSM, mockAssumedSSM, mockSTS)
			}

			store.readRoleArn = tt.readRoleArn
			store.secret = tt.secret
			got, err := store.Get(tt.stack, tt.component, tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("SSMStore.Get() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SSMStore.Get() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewSSMStore(t *testing.T) {
	// Check for AWS profile precondition
	tests.RequireAWSProfile(t, "cplive-core-gbl-identity")

	tests := []struct {
		name    string
		options SSMStoreOptions
		wantErr bool
	}{
		{
			name: "valid options with all fields",
			options: SSMStoreOptions{
				Prefix:         aws.String("/test-prefix"),
				Region:         "us-west-2",
				StackDelimiter: aws.String("/"),
				ReadRoleArn:    aws.String("arn:aws:iam::123456789012:role/read-role"),
				WriteRoleArn:   aws.String("arn:aws:iam::123456789012:role/write-role"),
			},
			wantErr: false,
		},
		{
			name: "valid options with required fields only",
			options: SSMStoreOptions{
				Region: "us-west-2",
			},
			wantErr: false,
		},
		{
			name: "missing region",
			options: SSMStoreOptions{
				Prefix: aws.String("/test-prefix"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear AWS_PROFILE to prevent conflicts with local AWS configuration.
			t.Setenv("AWS_PROFILE", "")

			store, err := NewSSMStore(tt.options, "")
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, store)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, store)

				ssmStore, ok := store.(*SSMStore)
				assert.True(t, ok)

				if tt.options.Prefix != nil {
					assert.Equal(t, *tt.options.Prefix, ssmStore.prefix)
				} else {
					assert.Equal(t, "", ssmStore.prefix)
				}

				if tt.options.StackDelimiter != nil {
					assert.Equal(t, *tt.options.StackDelimiter, *ssmStore.stackDelimiter)
				} else {
					assert.Equal(t, "-", *ssmStore.stackDelimiter)
				}

				assert.Equal(t, tt.options.Region, ssmStore.region)
				assert.Equal(t, tt.options.ReadRoleArn, ssmStore.readRoleArn)
				assert.Equal(t, tt.options.WriteRoleArn, ssmStore.writeRoleArn)
			}
		})
	}
}

func TestSSMStore_BuildAuthConfigOpts(t *testing.T) {
	tests := []struct {
		name          string
		region        string
		storeEndpoint string
		authContext   *storepkg.AWSAuthConfig
		wantLen       int
		// Expected applied effects after the built options are evaluated against a
		// config.LoadOptions. These assert precedence behavior, not just option count.
		wantRegion     string
		wantEndpoint   string
		wantProfile    string
		wantCredsFiles []string
		wantCfgFiles   []string
	}{
		{
			name:   "all fields populated",
			region: "us-east-1",
			authContext: &storepkg.AWSAuthConfig{
				CredentialsFile: "/path/to/creds",
				ConfigFile:      "/path/to/config",
				Profile:         "prod",
				Region:          "eu-west-1",
				EndpointURL:     "http://localhost:4566",
			},
			wantLen:        5,                       // creds + config + profile + store region + endpoint
			wantRegion:     "us-east-1",             // store region wins over auth context region
			wantEndpoint:   "http://localhost:4566", // store endpoint empty, so auth endpoint used
			wantProfile:    "prod",
			wantCredsFiles: []string{"/path/to/creds"},
			wantCfgFiles:   []string{"/path/to/config"},
		},
		{
			name:          "store endpoint used when auth context endpoint empty",
			region:        "us-east-1",
			storeEndpoint: "http://store-endpoint",
			authContext:   &storepkg.AWSAuthConfig{},
			wantLen:       2, // store region + store endpoint
			wantRegion:    "us-east-1",
			wantEndpoint:  "http://store-endpoint",
		},
		{
			name:          "store endpoint takes precedence over auth context endpoint",
			region:        "us-east-1",
			storeEndpoint: "http://store-endpoint",
			authContext: &storepkg.AWSAuthConfig{
				EndpointURL: "http://auth-endpoint",
			},
			wantLen:      2, // store region + store endpoint (auth endpoint ignored)
			wantRegion:   "us-east-1",
			wantEndpoint: "http://store-endpoint", // store endpoint wins over auth endpoint
		},
		{
			name:   "empty credentials file",
			region: "us-east-1",
			authContext: &storepkg.AWSAuthConfig{
				ConfigFile: "/path/to/config",
				Profile:    "prod",
			},
			wantLen:      3, // config + profile + region
			wantRegion:   "us-east-1",
			wantProfile:  "prod",
			wantCfgFiles: []string{"/path/to/config"},
		},
		{
			name:   "region fallback from auth context",
			region: "",
			authContext: &storepkg.AWSAuthConfig{
				CredentialsFile: "/path/to/creds",
				Region:          "eu-west-1",
			},
			wantLen:        2,           // creds + auth region
			wantRegion:     "eu-west-1", // store region empty, so auth context region used
			wantCredsFiles: []string{"/path/to/creds"},
		},
		{
			name:        "all empty auth context with store region",
			region:      "us-east-1",
			authContext: &storepkg.AWSAuthConfig{},
			wantLen:     1, // just store region
			wantRegion:  "us-east-1",
		},
		{
			name:        "both regions empty",
			region:      "",
			authContext: &storepkg.AWSAuthConfig{},
			wantLen:     0,
		},
		{
			name:   "only profile set",
			region: "",
			authContext: &storepkg.AWSAuthConfig{
				Profile: "prod-admin",
			},
			wantLen:     1, // just profile
			wantProfile: "prod-admin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &SSMStore{region: tt.region, endpoint: tt.storeEndpoint}
			opts := store.buildAuthConfigOpts(tt.authContext)
			assert.Len(t, opts, tt.wantLen)

			// Apply the built options and assert their effect, so endpoint/region/profile
			// precedence cannot regress while still passing the length check.
			var lo config.LoadOptions
			for _, opt := range opts {
				require.NoError(t, opt(&lo))
			}
			assert.Equal(t, tt.wantRegion, lo.Region)
			assert.Equal(t, tt.wantEndpoint, lo.BaseEndpoint)
			assert.Equal(t, tt.wantProfile, lo.SharedConfigProfile)
			assert.Equal(t, tt.wantCredsFiles, lo.SharedCredentialsFiles)
			assert.Equal(t, tt.wantCfgFiles, lo.SharedConfigFiles)
		})
	}
}

func TestNewSSMStore_EndpointFallback(t *testing.T) {
	ptr := func(s string) *string { return &s }

	tests := []struct {
		name         string
		endpoint     *string
		endpointURL  *string
		wantEndpoint string
	}{
		{
			name:         "no endpoint configured",
			wantEndpoint: "",
		},
		{
			name:         "endpoint takes precedence over endpoint_url",
			endpoint:     ptr("http://endpoint"),
			endpointURL:  ptr("http://endpoint-url"),
			wantEndpoint: "http://endpoint",
		},
		{
			name:         "endpoint_url used when endpoint nil",
			endpointURL:  ptr("http://endpoint-url"),
			wantEndpoint: "http://endpoint-url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewSSMStore(SSMStoreOptions{
				Region:      "us-east-1",
				Endpoint:    tt.endpoint,
				EndpointURL: tt.endpointURL,
			}, "")
			require.NoError(t, err)
			ssmStore, ok := s.(*SSMStore)
			require.True(t, ok)
			assert.Equal(t, tt.wantEndpoint, ssmStore.endpoint)
		})
	}
}

func TestSSMStore_InitDefaultClient_WithEndpoint(t *testing.T) {
	store := &SSMStore{region: "us-east-1", endpoint: "http://localhost:4566"}

	require.NoError(t, store.initDefaultClient())
	assert.NotNil(t, store.client)
}

func TestSSMStore_getKey(t *testing.T) {
	tests := []struct {
		name           string
		prefix         string
		stackDelimiter *string
		stack          string
		component      string
		key            string
		want           string
		wantErr        bool
	}{
		{
			name:           "valid key with prefix and forward slash delimiter",
			prefix:         "/test-prefix",
			stackDelimiter: aws.String("/"),
			stack:          "dev/usw2/app",
			component:      "service",
			key:            "config-key",
			want:           "/test-prefix/dev/usw2/app/service/config-key",
			wantErr:        false,
		},
		{
			name:           "valid key with no prefix and hyphen delimiter",
			prefix:         "",
			stackDelimiter: aws.String("-"),
			stack:          "dev/usw2/app",
			component:      "service",
			key:            "config-key",
			want:           "/dev/usw2/app/service/config-key",
			wantErr:        false,
		},
		{
			name:           "nil stack delimiter",
			prefix:         "/test-prefix",
			stackDelimiter: nil,
			stack:          "dev/usw2/app",
			component:      "service",
			key:            "config-key",
			want:           "",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &SSMStore{
				prefix:         tt.prefix,
				stackDelimiter: tt.stackDelimiter,
			}

			got, err := s.getKey(tt.stack, tt.component, tt.key)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, tt.want, got)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestSSMStore_GetKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockSSM := NewMockSSMClient(ctrl)
	mockSTS := NewMockSTSClient(ctrl)
	mockAssumedSSM := NewMockSSMClient(ctrl)
	stackDelimiter := "/"
	awsConfig := &aws.Config{Region: "us-west-2"}

	store := &SSMStore{
		client:         mockSSM,
		prefix:         "/test-prefix",
		stackDelimiter: &stackDelimiter,
		awsConfig:      awsConfig,
		newSTSClient: func(cfg aws.Config) STSClient {
			return mockSTS
		},
		newSSMClient: func(cfg aws.Config) SSMClient {
			return mockAssumedSSM
		},
	}

	tests := []struct {
		name        string
		key         string
		secret      bool
		readRoleArn *string
		mockSetup   func(*MockSSMClient, *MockSSMClient, *MockSTSClient)
		want        interface{}
		wantErr     bool
	}{
		{
			// GetKey shares decodeParameterValue with Get: secret-store JSON scalars
			// that are not strings return byte-exact instead of being re-typed.
			name:   "secret_raw_number_string_round_trips",
			key:    "dev/usw2/app/service/secret-key",
			secret: true,
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/secret-key"),
					WithDecryption: aws.Bool(true),
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`1e5`),
					},
				}, nil)
			},
			want: "1e5",
		},
		{
			name: "successful_get",
			key:  "dev/usw2/app/service/config-key",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/config-key"),
					WithDecryption: aws.Bool(true),
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`"test-value"`),
					},
				}, nil)
			},
			want: "test-value",
		},
		{
			name: "successful_get_slice",
			key:  "dev/usw2/app/service/slice-key",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/slice-key"),
					WithDecryption: aws.Bool(true),
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`["value1","value2","value3"]`),
					},
				}, nil)
			},
			want: []interface{}{"value1", "value2", "value3"},
		},
		{
			name: "successful_get_map",
			key:  "dev/usw2/app/service/map-key",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/map-key"),
					WithDecryption: aws.Bool(true),
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`{"key1":"value1","key2":42}`),
					},
				}, nil)
			},
			want: map[string]interface{}{"key1": "value1", "key2": float64(42)},
		},
		{
			name: "aws_error",
			key:  "dev/usw2/app/service/config-key",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), gomock.Any()).Return(nil, errors.New("aws error"))
			},
			wantErr: true,
		},
		{
			name:      "empty_key",
			key:       "",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {},
			wantErr:   true,
		},
		{
			name: "parameter_not_found",
			key:  "dev/usw2/app/service/non-existent-key",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), gomock.Any()).Return(nil, &types.ParameterNotFound{})
			},
			wantErr: true,
		},
		{
			name: "non-json_value",
			key:  "dev/usw2/app/service/plain-text-key",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/plain-text-key"),
					WithDecryption: aws.Bool(true),
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String("plain text value"),
					},
				}, nil)
			},
			want:    "plain text value",
			wantErr: false,
		},
		{
			name: "malformed_json_value",
			key:  "dev/usw2/app/service/malformed-json-key",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/malformed-json-key"),
					WithDecryption: aws.Bool(true),
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String("}invalid json{"),
					},
				}, nil)
			},
			want:    "}invalid json{",
			wantErr: false,
		},
		{
			name: "integer_value",
			key:  "dev/usw2/app/service/integer-key",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/integer-key"),
					WithDecryption: aws.Bool(true),
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`42`),
					},
				}, nil)
			},
			want: float64(42),
		},
		{
			name: "float_value",
			key:  "dev/usw2/app/service/float-key",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/float-key"),
					WithDecryption: aws.Bool(true),
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`3.14159`),
					},
				}, nil)
			},
			want: float64(3.14159),
		},
		{
			name: "numeric_string",
			key:  "dev/usw2/app/service/numeric-string-key",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/numeric-string-key"),
					WithDecryption: aws.Bool(true),
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`"42"`),
					},
				}, nil)
			},
			want: "42",
		},
		{
			name:        "successful_get_with_read_role",
			key:         "dev/usw2/app/service/config-key",
			readRoleArn: aws.String("arn:aws:iam::123456789012:role/read-role"),
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSTS.EXPECT().AssumeRole(gomock.Any(), &sts.AssumeRoleInput{
					RoleArn:         aws.String("arn:aws:iam::123456789012:role/read-role"),
					RoleSessionName: aws.String("atmos-ssm-session"),
				}).Return(&sts.AssumeRoleOutput{
					Credentials: &ststypes.Credentials{
						AccessKeyId:     aws.String("AKIATEST"),
						SecretAccessKey: aws.String("secret"),
						SessionToken:    aws.String("token"),
					},
				}, nil)

				mockAssumedSSM.EXPECT().GetParameter(gomock.Any(), &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/config-key"),
					WithDecryption: aws.Bool(true),
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`"test-value"`),
					},
				}, nil)
			},
			want: "test-value",
		},
		{
			name:        "failed_role_assumption_for_read",
			key:         "dev/usw2/app/service/config-key",
			readRoleArn: aws.String("arn:aws:iam::123456789012:role/read-role"),
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSTS.EXPECT().AssumeRole(gomock.Any(), &sts.AssumeRoleInput{
					RoleArn:         aws.String("arn:aws:iam::123456789012:role/read-role"),
					RoleSessionName: aws.String("atmos-ssm-session"),
				}).Return(nil, fmt.Errorf("failed to assume role"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mockSetup != nil {
				tt.mockSetup(mockSSM, mockAssumedSSM, mockSTS)
			}

			store.readRoleArn = tt.readRoleArn
			store.secret = tt.secret
			got, err := store.GetKey(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("SSMStore.GetKey() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SSMStore.GetKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewSSMStore_EmptyRegion(t *testing.T) {
	_, err := NewSSMStore(SSMStoreOptions{}, "")
	assert.ErrorIs(t, err, storepkg.ErrRegionRequired)
}

func TestNewSSMStore_WithRolesAndDelimiter(t *testing.T) {
	// Pass a non-empty identity so client initialization is deferred (lazy):
	// the constructor skips initDefaultClient when an identity is set, so this
	// test is fully deterministic with no AWS dependency while still exercising
	// the option-to-field assignment branches (delimiter, read/write role ARNs).
	store, err := NewSSMStore(SSMStoreOptions{
		Region:         "us-east-1",
		StackDelimiter: stringPtr("|"),
		ReadRoleArn:    stringPtr("arn:aws:iam::123456789012:role/read"),
		WriteRoleArn:   stringPtr("arn:aws:iam::123456789012:role/write"),
	}, "test-identity")
	require.NoError(t, err)

	ssmStore := store.(*SSMStore)
	assert.Equal(t, "|", *ssmStore.stackDelimiter)
	assert.Equal(t, "arn:aws:iam::123456789012:role/read", *ssmStore.readRoleArn)
	assert.Equal(t, "arn:aws:iam::123456789012:role/write", *ssmStore.writeRoleArn)
	assert.Nil(t, ssmStore.client) // Lazy init — no client created when an identity is configured.
}

func TestSSMStore_assumeRole_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockSTS := NewMockSTSClient(ctrl)
	mockSTS.EXPECT().AssumeRole(gomock.Any(), gomock.Any()).Return(nil, errors.New("assume boom"))

	stackDelimiter := "/"
	store := &SSMStore{
		client:         NewMockSSMClient(ctrl),
		stackDelimiter: &stackDelimiter,
		awsConfig:      &aws.Config{Region: "us-west-2"},
		writeRoleArn:   aws.String("arn:aws:iam::123456789012:role/test"),
		newSTSClient:   func(_ aws.Config) STSClient { return mockSTS },
		newSSMClient:   func(_ aws.Config) SSMClient { return NewMockSSMClient(ctrl) },
	}

	err := store.Set("dev", "app", "k", "v")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to assume")
}

func TestSSMStore_Set_MoreErrors(t *testing.T) {
	newStore := func(t *testing.T) *SSMStore {
		t.Helper()
		ctrl := gomock.NewController(t)
		delimiter := "/"
		return &SSMStore{
			client:         NewMockSSMClient(ctrl),
			stackDelimiter: &delimiter,
			awsConfig:      &aws.Config{Region: "us-west-2"},
		}
	}

	t.Run("nil value", func(t *testing.T) {
		assert.ErrorIs(t, newStore(t).Set("dev", "app", "k", nil), storepkg.ErrNilValue)
	})

	t.Run("marshal error", func(t *testing.T) {
		assert.ErrorIs(t, newStore(t).Set("dev", "app", "k", make(chan int)), storepkg.ErrSerializeJSON)
	})
}

func TestSSMStore_GetKey_PrefixAddsLeadingSlash(t *testing.T) {
	delimiter := "/"
	ctrl := gomock.NewController(t)
	mockSSM := NewMockSSMClient(ctrl)
	// Prefix without a leading slash exercises the "/"-prepend branch.
	mockSSM.EXPECT().GetParameter(gomock.Any(), gomock.Cond(func(in *ssm.GetParameterInput) bool {
		return *in.Name == "/myprefix/k"
	})).Return(&ssm.GetParameterOutput{
		Parameter: &types.Parameter{Value: aws.String(`"val"`)},
	}, nil)

	store := &SSMStore{
		client:         mockSSM,
		prefix:         "myprefix",
		stackDelimiter: &delimiter,
		awsConfig:      &aws.Config{Region: "us-west-2"},
	}

	v, err := store.GetKey("k")
	assert.NoError(t, err)
	assert.Equal(t, "val", v)
}

func TestBuildSSMStore_ParseError(t *testing.T) {
	_, err := buildSSMStore("n", storepkg.StoreConfig{
		Options: map[string]interface{}{"region": []string{"x"}},
	})
	assert.ErrorIs(t, err, storepkg.ErrParseSSMOptions)
}

// TestSSMStore_Keys proves Keys paginates via GetParametersByPath (WithDecryption=false, no
// kms:Decrypt needed) and strips the queried path from each returned parameter name.
func TestSSMStore_Keys(t *testing.T) {
	stackDelimiter := "-"
	ctrl := gomock.NewController(t)
	mockSSM := NewMockSSMClient(ctrl)
	store := &SSMStore{client: mockSSM, prefix: "/atmos", stackDelimiter: &stackDelimiter, awsConfig: &aws.Config{Region: "us-east-1"}}

	mockSSM.EXPECT().GetParametersByPath(gomock.Any(), &ssm.GetParametersByPathInput{
		Path:           aws.String("/atmos/prod/vpc"),
		Recursive:      aws.Bool(true),
		WithDecryption: aws.Bool(false),
		NextToken:      (*string)(nil),
	}).Return(&ssm.GetParametersByPathOutput{
		Parameters: []types.Parameter{{Name: aws.String("/atmos/prod/vpc/image_tag")}},
		NextToken:  aws.String("page2"),
	}, nil)
	mockSSM.EXPECT().GetParametersByPath(gomock.Any(), &ssm.GetParametersByPathInput{
		Path:           aws.String("/atmos/prod/vpc"),
		Recursive:      aws.Bool(true),
		WithDecryption: aws.Bool(false),
		NextToken:      aws.String("page2"),
	}).Return(&ssm.GetParametersByPathOutput{
		Parameters: []types.Parameter{{Name: aws.String("/atmos/prod/vpc/region")}},
	}, nil)

	keys, err := store.Keys("prod", "vpc")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"image_tag", "region"}, keys)
}

func TestSSMStore_Keys_Error(t *testing.T) {
	stackDelimiter := "-"
	ctrl := gomock.NewController(t)
	mockSSM := NewMockSSMClient(ctrl)
	store := &SSMStore{client: mockSSM, prefix: "/atmos", stackDelimiter: &stackDelimiter, awsConfig: &aws.Config{Region: "us-east-1"}}

	mockSSM.EXPECT().GetParametersByPath(gomock.Any(), gomock.Any()).Return((*ssm.GetParametersByPathOutput)(nil), assert.AnError)

	_, err := store.Keys("prod", "vpc")
	require.Error(t, err)
	assert.ErrorIs(t, err, storepkg.ErrListParameters)
}
