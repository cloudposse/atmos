package store

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	ststypes "github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/cloudposse/atmos/tests"
)

// MockSSMClient is a mock implementation of the SSMClient interface.
type MockSSMClient struct {
	mock.Mock
}

func (m *MockSSMClient) PutParameter(ctx context.Context, params *ssm.PutParameterInput, optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ssm.PutParameterOutput), args.Error(1)
}

func (m *MockSSMClient) GetParameter(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ssm.GetParameterOutput), args.Error(1)
}

// MockSTSClient is a mock implementation of the STSClient interface.
type MockSTSClient struct {
	mock.Mock
}

func (m *MockSTSClient) AssumeRole(ctx context.Context, params *sts.AssumeRoleInput, optFns ...func(*sts.Options)) (*sts.AssumeRoleOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*sts.AssumeRoleOutput), args.Error(1)
}

func TestSSMStore_Set(t *testing.T) {
	mockSSM := new(MockSSMClient)
	mockSTS := new(MockSTSClient)
	mockAssumedSSM := new(MockSSMClient)
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
		name         string
		stack        string
		component    string
		key          string
		value        interface{}
		writeRoleArn *string
		mockSetup    func(*MockSSMClient, *MockSSMClient, *MockSTSClient)
		wantErr      bool
	}{
		{
			name:      "successful_set",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "config-key",
			value:     "test-value",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.On("PutParameter", mock.Anything, &ssm.PutParameterInput{
					Name:      aws.String("/test-prefix/dev/usw2/app/service/config-key"),
					Value:     aws.String(`"test-value"`),
					Type:      types.ParameterTypeString,
					Overwrite: aws.Bool(true),
				}).Return(&ssm.PutParameterOutput{}, nil)
			},
		},
		{
			name:      "successful_set_with_slice",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "slice-key",
			value:     []string{"value1", "value2", "value3"},
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.On("PutParameter", mock.Anything, &ssm.PutParameterInput{
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
				mockSSM.On("PutParameter", mock.Anything, &ssm.PutParameterInput{
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
				mockSSM.On("PutParameter", mock.Anything, mock.MatchedBy(func(input *ssm.PutParameterInput) bool {
					return true
				})).Return(nil, errors.New("aws error"))
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
				mockSSM.On("PutParameter", mock.Anything, mock.MatchedBy(func(input *ssm.PutParameterInput) bool {
					return true
				})).Return(nil, errors.New("invalid value type"))
			},
			wantErr: true,
		},
		{
			name:      "empty_stack",
			stack:     "",
			component: "service",
			key:       "config-key",
			value:     "test-value",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {},
			wantErr:   true,
		},
		{
			name:      "empty_component",
			stack:     "dev/usw2/app",
			component: "",
			key:       "config-key",
			value:     "test-value",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {},
			wantErr:   true,
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
			name:      "nil_stack_delimiter",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "config-key",
			value:     "test-value",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.On("PutParameter", mock.Anything, &ssm.PutParameterInput{
					Name:      aws.String("/test-prefix/dev/usw2/app/service/config-key"),
					Value:     aws.String(`"test-value"`),
					Type:      types.ParameterTypeString,
					Overwrite: aws.Bool(true),
				}).Return(&ssm.PutParameterOutput{}, nil)
			},
		},
		{
			name:      "complex_stack_name_with_multiple_delimiters",
			stack:     "dev/usw2/prod/app",
			component: "service",
			key:       "config-key",
			value:     "test-value",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.On("PutParameter", mock.Anything, &ssm.PutParameterInput{
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
				mockSTS.On("AssumeRole", mock.Anything, &sts.AssumeRoleInput{
					RoleArn:         aws.String("arn:aws:iam::123456789012:role/write-role"),
					RoleSessionName: aws.String("atmos-ssm-session"),
				}).Return(&sts.AssumeRoleOutput{
					Credentials: &ststypes.Credentials{
						AccessKeyId:     aws.String("AKIATEST"),
						SecretAccessKey: aws.String("secret"),
						SessionToken:    aws.String("token"),
					},
				}, nil)

				mockAssumedSSM.On("PutParameter", mock.Anything, &ssm.PutParameterInput{
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
				mockSTS.On("AssumeRole", mock.Anything, &sts.AssumeRoleInput{
					RoleArn:         aws.String("arn:aws:iam::123456789012:role/write-role"),
					RoleSessionName: aws.String("atmos-ssm-session"),
				}).Return(nil, fmt.Errorf("failed to assume role"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSSM.ExpectedCalls = nil
			mockAssumedSSM.ExpectedCalls = nil
			mockSTS.ExpectedCalls = nil
			if tt.mockSetup != nil {
				tt.mockSetup(mockSSM, mockAssumedSSM, mockSTS)
			}

			store.writeRoleArn = tt.writeRoleArn
			err := store.Set(tt.stack, tt.component, tt.key, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("SSMStore.Set() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			mockSSM.AssertExpectations(t)
			mockAssumedSSM.AssertExpectations(t)
			mockSTS.AssertExpectations(t)
		})
	}
}

func TestSSMStore_Get(t *testing.T) {
	mockSSM := new(MockSSMClient)
	mockSTS := new(MockSTSClient)
	mockAssumedSSM := new(MockSSMClient)
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
				mockSSM.On("GetParameter", mock.Anything, &ssm.GetParameterInput{
					Name: aws.String("/test-prefix/dev/usw2/app/service/config-key"),
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
				mockSSM.On("GetParameter", mock.Anything, &ssm.GetParameterInput{
					Name: aws.String("/test-prefix/dev/usw2/app/service/slice-key"),
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
				mockSSM.On("GetParameter", mock.Anything, &ssm.GetParameterInput{
					Name: aws.String("/test-prefix/dev/usw2/app/service/map-key"),
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
				mockSSM.On("GetParameter", mock.Anything, mock.MatchedBy(func(input *ssm.GetParameterInput) bool {
					return true
				})).Return(nil, errors.New("aws error"))
			},
			wantErr: true,
		},
		{
			name:      "empty_stack",
			stack:     "",
			component: "service",
			key:       "config-key",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {},
			wantErr:   true,
		},
		{
			name:      "empty_component",
			stack:     "dev/usw2/app",
			component: "",
			key:       "config-key",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {},
			wantErr:   true,
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
				mockSSM.On("GetParameter", mock.Anything, mock.MatchedBy(func(input *ssm.GetParameterInput) bool {
					return true
				})).Return(nil, &types.ParameterNotFound{})
			},
			wantErr: true,
		},
		{
			name:      "non-json_value",
			stack:     "dev/usw2/app",
			component: "service",
			key:       "plain-text-key",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.On("GetParameter", mock.Anything, &ssm.GetParameterInput{
					Name: aws.String("/test-prefix/dev/usw2/app/service/plain-text-key"),
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
				mockSSM.On("GetParameter", mock.Anything, &ssm.GetParameterInput{
					Name: aws.String("/test-prefix/dev/usw2/app/service/malformed-json-key"),
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
				mockSSM.On("GetParameter", mock.Anything, &ssm.GetParameterInput{
					Name: aws.String("/test-prefix/dev/usw2/app/service/integer-key"),
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
				mockSSM.On("GetParameter", mock.Anything, &ssm.GetParameterInput{
					Name: aws.String("/test-prefix/dev/usw2/app/service/float-key"),
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
				mockSSM.On("GetParameter", mock.Anything, &ssm.GetParameterInput{
					Name: aws.String("/test-prefix/dev/usw2/app/service/numeric-string-key"),
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
			stack:       "dev/usw2/app",
			component:   "service",
			key:         "config-key",
			readRoleArn: aws.String("arn:aws:iam::123456789012:role/read-role"),
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSTS.On("AssumeRole", mock.Anything, &sts.AssumeRoleInput{
					RoleArn:         aws.String("arn:aws:iam::123456789012:role/read-role"),
					RoleSessionName: aws.String("atmos-ssm-session"),
				}).Return(&sts.AssumeRoleOutput{
					Credentials: &ststypes.Credentials{
						AccessKeyId:     aws.String("AKIATEST"),
						SecretAccessKey: aws.String("secret"),
						SessionToken:    aws.String("token"),
					},
				}, nil)

				mockAssumedSSM.On("GetParameter", mock.Anything, &ssm.GetParameterInput{
					Name: aws.String("/test-prefix/dev/usw2/app/service/config-key"),
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
				mockSTS.On("AssumeRole", mock.Anything, &sts.AssumeRoleInput{
					RoleArn:         aws.String("arn:aws:iam::123456789012:role/read-role"),
					RoleSessionName: aws.String("atmos-ssm-session"),
				}).Return(nil, fmt.Errorf("failed to assume role"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSSM.ExpectedCalls = nil
			mockAssumedSSM.ExpectedCalls = nil
			mockSTS.ExpectedCalls = nil
			if tt.mockSetup != nil {
				tt.mockSetup(mockSSM, mockAssumedSSM, mockSTS)
			}

			store.readRoleArn = tt.readRoleArn
			got, err := store.Get(tt.stack, tt.component, tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("SSMStore.Get() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SSMStore.Get() = %v, want %v", got, tt.want)
			}
			mockSSM.AssertExpectations(t)
			mockSTS.AssertExpectations(t)
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

			store, err := NewSSMStore(tt.options)
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

				assert.Equal(t, tt.options.Region, ssmStore.awsConfig.Region)
				assert.Equal(t, tt.options.ReadRoleArn, ssmStore.readRoleArn)
				assert.Equal(t, tt.options.WriteRoleArn, ssmStore.writeRoleArn)
			}
		})
	}
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
	mockSSM := new(MockSSMClient)
	mockSTS := new(MockSTSClient)
	mockAssumedSSM := new(MockSSMClient)
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
		readRoleArn *string
		mockSetup   func(*MockSSMClient, *MockSSMClient, *MockSTSClient)
		want        interface{}
		wantErr     bool
	}{
		{
			name: "successful_get",
			key:  "dev/usw2/app/service/config-key",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.On("GetParameter", mock.Anything, &ssm.GetParameterInput{
					Name: aws.String("/dev/usw2/app/service/config-key"),
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
				mockSSM.On("GetParameter", mock.Anything, &ssm.GetParameterInput{
					Name: aws.String("/dev/usw2/app/service/slice-key"),
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
				mockSSM.On("GetParameter", mock.Anything, &ssm.GetParameterInput{
					Name: aws.String("/dev/usw2/app/service/map-key"),
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
				mockSSM.On("GetParameter", mock.Anything, mock.MatchedBy(func(input *ssm.GetParameterInput) bool {
					return true
				})).Return(nil, errors.New("aws error"))
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
				mockSSM.On("GetParameter", mock.Anything, mock.MatchedBy(func(input *ssm.GetParameterInput) bool {
					return true
				})).Return(nil, &types.ParameterNotFound{})
			},
			wantErr: true,
		},
		{
			name: "non-json_value",
			key:  "dev/usw2/app/service/plain-text-key",
			mockSetup: func(mockSSM *MockSSMClient, mockAssumedSSM *MockSSMClient, mockSTS *MockSTSClient) {
				mockSSM.On("GetParameter", mock.Anything, &ssm.GetParameterInput{
					Name: aws.String("/dev/usw2/app/service/plain-text-key"),
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
				mockSSM.On("GetParameter", mock.Anything, &ssm.GetParameterInput{
					Name: aws.String("/dev/usw2/app/service/malformed-json-key"),
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
				mockSSM.On("GetParameter", mock.Anything, &ssm.GetParameterInput{
					Name: aws.String("/dev/usw2/app/service/integer-key"),
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
				mockSSM.On("GetParameter", mock.Anything, &ssm.GetParameterInput{
					Name: aws.String("/dev/usw2/app/service/float-key"),
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
				mockSSM.On("GetParameter", mock.Anything, &ssm.GetParameterInput{
					Name: aws.String("/dev/usw2/app/service/numeric-string-key"),
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
				mockSTS.On("AssumeRole", mock.Anything, &sts.AssumeRoleInput{
					RoleArn:         aws.String("arn:aws:iam::123456789012:role/read-role"),
					RoleSessionName: aws.String("atmos-ssm-session"),
				}).Return(&sts.AssumeRoleOutput{
					Credentials: &ststypes.Credentials{
						AccessKeyId:     aws.String("AKIATEST"),
						SecretAccessKey: aws.String("secret"),
						SessionToken:    aws.String("token"),
					},
				}, nil)

				mockAssumedSSM.On("GetParameter", mock.Anything, &ssm.GetParameterInput{
					Name: aws.String("/dev/usw2/app/service/config-key"),
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
				mockSTS.On("AssumeRole", mock.Anything, &sts.AssumeRoleInput{
					RoleArn:         aws.String("arn:aws:iam::123456789012:role/read-role"),
					RoleSessionName: aws.String("atmos-ssm-session"),
				}).Return(nil, fmt.Errorf("failed to assume role"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSSM.ExpectedCalls = nil
			mockAssumedSSM.ExpectedCalls = nil
			mockSTS.ExpectedCalls = nil
			if tt.mockSetup != nil {
				tt.mockSetup(mockSSM, mockAssumedSSM, mockSTS)
			}

			store.readRoleArn = tt.readRoleArn
			got, err := store.GetKey(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("SSMStore.GetKey() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SSMStore.GetKey() = %v, want %v", got, tt.want)
			}
			mockSSM.AssertExpectations(t)
			mockSTS.AssertExpectations(t)
		})
	}
}
