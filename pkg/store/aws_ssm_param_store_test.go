package store

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockSSMClient is a mock implementation of SSMClient
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

func TestSSMStore_Set(t *testing.T) {
	mockFnOverwrite := true
	testPrefix := "/test-prefix"
	testDelimiter := "-"

	tests := []struct {
		name      string
		stack     string
		component string
		key       string
		value     interface{}
		mockFn    func(*MockSSMClient)
		wantErr   bool
	}{
		{
			name:      "successful set",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "config-key",
			value:     "test-value",
			mockFn: func(m *MockSSMClient) {
				m.On("PutParameter", mock.Anything, &ssm.PutParameterInput{
					Name:      aws.String("/test-prefix/dev/usw2/app/service/config-key"),
					Value:     aws.String(`"test-value"`),
					Type:      types.ParameterTypeString,
					Overwrite: &mockFnOverwrite,
				}).Return(&ssm.PutParameterOutput{}, nil)
			},
			wantErr: false,
		},
		{
			name:      "successful set with slice",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "slice-key",
			value:     []string{"value1", "value2", "value3"},
			mockFn: func(m *MockSSMClient) {
				m.On("PutParameter", mock.Anything, &ssm.PutParameterInput{
					Name:      aws.String("/test-prefix/dev/usw2/app/service/slice-key"),
					Value:     aws.String(`["value1","value2","value3"]`),
					Type:      types.ParameterTypeString,
					Overwrite: &mockFnOverwrite,
				}).Return(&ssm.PutParameterOutput{}, nil)
			},
			wantErr: false,
		},
		{
			name:      "successful set with map",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "map-key",
			value:     map[string]interface{}{"key1": "value1", "key2": 42, "key3": true},
			mockFn: func(m *MockSSMClient) {
				m.On("PutParameter", mock.Anything, &ssm.PutParameterInput{
					Name:      aws.String("/test-prefix/dev/usw2/app/service/map-key"),
					Value:     aws.String(`{"key1":"value1","key2":42,"key3":true}`),
					Type:      types.ParameterTypeString,
					Overwrite: &mockFnOverwrite,
				}).Return(&ssm.PutParameterOutput{}, nil)
			},
			wantErr: false,
		},
		{
			name:      "aws error",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "config-key",
			value:     "test-value",
			mockFn: func(m *MockSSMClient) {
				m.On("PutParameter", mock.Anything, mock.Anything).
					Return(nil, errors.New("aws error")) //nolint
			},
			wantErr: true,
		},
		{
			name:      "invalid value type",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "config-key",
			value:     123, // Not a string
			mockFn: func(m *MockSSMClient) {
				m.On("PutParameter", mock.Anything, mock.Anything).
					Return(nil, errors.New("invalid value type")) //nolint
			},
			wantErr: true,
		},
		{
			name:      "empty stack",
			stack:     "",
			component: "app/service",
			key:       "config-key",
			value:     "test-value",
			mockFn:    func(m *MockSSMClient) {},
			wantErr:   true,
		},
		{
			name:      "empty component",
			stack:     "dev-usw2",
			component: "",
			key:       "config-key",
			value:     "test-value",
			mockFn:    func(m *MockSSMClient) {},
			wantErr:   true,
		},
		{
			name:      "empty key",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "",
			value:     "test-value",
			mockFn:    func(m *MockSSMClient) {},
			wantErr:   true,
		},
		{
			name:      "nil stack delimiter",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "config-key",
			value:     "test-value",
			mockFn: func(m *MockSSMClient) {
				m.On("PutParameter", mock.Anything, &ssm.PutParameterInput{
					Name:      aws.String("/test-prefix/dev/usw2/app/service/config-key"),
					Value:     aws.String(`"test-value"`),
					Type:      types.ParameterTypeString,
					Overwrite: &mockFnOverwrite,
				}).Return(&ssm.PutParameterOutput{}, nil)
			},
			wantErr: false,
		},
		{
			name:      "complex stack name with multiple delimiters",
			stack:     "dev-usw2-prod",
			component: "app/service",
			key:       "config-key",
			value:     "test-value",
			mockFn: func(m *MockSSMClient) {
				m.On("PutParameter", mock.Anything, &ssm.PutParameterInput{
					Name:      aws.String("/test-prefix/dev/usw2/prod/app/service/config-key"),
					Value:     aws.String(`"test-value"`),
					Type:      types.ParameterTypeString,
					Overwrite: &mockFnOverwrite,
				}).Return(&ssm.PutParameterOutput{}, nil)
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(MockSSMClient)
			tt.mockFn(mockClient)

			store := &SSMStore{
				client:         mockClient,
				prefix:         testPrefix,
				stackDelimiter: aws.String(testDelimiter),
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

func TestSSMStore_Get(t *testing.T) {
	mockFnWithDecryption := true
	testPrefix := "/test-prefix"
	testDelimiter := "-"

	tests := []struct {
		name      string
		stack     string
		component string
		key       string
		mockFn    func(*MockSSMClient)
		want      interface{}
		wantErr   bool
	}{
		{
			name:      "successful get",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "config-key",
			mockFn: func(m *MockSSMClient) {
				m.On("GetParameter", mock.Anything, &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/config-key"),
					WithDecryption: &mockFnWithDecryption,
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`"test-value"`),
					},
				}, nil)
			},
			want:    "test-value",
			wantErr: false,
		},
		{
			name:      "successful get slice",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "slice-key",
			mockFn: func(m *MockSSMClient) {
				m.On("GetParameter", mock.Anything, &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/slice-key"),
					WithDecryption: &mockFnWithDecryption,
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`["value1","value2","value3"]`),
					},
				}, nil)
			},
			want:    []interface{}{"value1", "value2", "value3"},
			wantErr: false,
		},
		{
			name:      "successful get map",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "map-key",
			mockFn: func(m *MockSSMClient) {
				m.On("GetParameter", mock.Anything, &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/map-key"),
					WithDecryption: &mockFnWithDecryption,
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`{"key1":"value1","key2":"value2"}`),
					},
				}, nil)
			},
			want:    map[string]interface{}{"key1": "value1", "key2": "value2"},
			wantErr: false,
		},
		{
			name:      "aws error",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "config-key",
			mockFn: func(m *MockSSMClient) {
				m.On("GetParameter", mock.Anything, mock.Anything).
					Return(nil, errors.New("aws error")) //nolint
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:      "empty stack",
			stack:     "",
			component: "app/service",
			key:       "config-key",
			mockFn:    func(m *MockSSMClient) {},
			want:      nil,
			wantErr:   true,
		},
		{
			name:      "empty component",
			stack:     "dev-usw2",
			component: "",
			key:       "config-key",
			mockFn:    func(m *MockSSMClient) {},
			want:      nil,
			wantErr:   true,
		},
		{
			name:      "empty key",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "",
			mockFn:    func(m *MockSSMClient) {},
			want:      nil,
			wantErr:   true,
		},
		{
			name:      "parameter not found",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "non-existent-key",
			mockFn: func(m *MockSSMClient) {
				m.On("GetParameter", mock.Anything, mock.Anything).
					Return(nil, &types.ParameterNotFound{})
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:      "non-json value",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "plain-text-key",
			mockFn: func(m *MockSSMClient) {
				m.On("GetParameter", mock.Anything, &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/plain-text-key"),
					WithDecryption: &mockFnWithDecryption,
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`plain text value`),
					},
				}, nil)
			},
			want:    "plain text value",
			wantErr: false,
		},
		{
			name:      "malformed json value",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "malformed-json-key",
			mockFn: func(m *MockSSMClient) {
				m.On("GetParameter", mock.Anything, &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/malformed-json-key"),
					WithDecryption: &mockFnWithDecryption,
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`{"key1":"value1", "key2":}`),
					},
				}, nil)
			},
			want:    `{"key1":"value1", "key2":}`,
			wantErr: false,
		},
		{
			name:      "integer value",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "integer-key",
			mockFn: func(m *MockSSMClient) {
				m.On("GetParameter", mock.Anything, &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/integer-key"),
					WithDecryption: &mockFnWithDecryption,
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`42`),
					},
				}, nil)
			},
			want:    float64(42), // JSON unmarshals numbers as float64
			wantErr: false,
		},
		{
			name:      "float value",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "float-key",
			mockFn: func(m *MockSSMClient) {
				m.On("GetParameter", mock.Anything, &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/float-key"),
					WithDecryption: &mockFnWithDecryption,
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`3.14159`),
					},
				}, nil)
			},
			want:    3.14159,
			wantErr: false,
		},
		{
			name:      "numeric string",
			stack:     "dev-usw2",
			component: "app/service",
			key:       "numeric-string-key",
			mockFn: func(m *MockSSMClient) {
				m.On("GetParameter", mock.Anything, &ssm.GetParameterInput{
					Name:           aws.String("/test-prefix/dev/usw2/app/service/numeric-string-key"),
					WithDecryption: &mockFnWithDecryption,
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String(`"42"`), // JSON string containing a number
					},
				}, nil)
			},
			want:    "42", // Should be parsed as a string, not a number
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(MockSSMClient)
			tt.mockFn(mockClient)

			store := &SSMStore{
				client:         mockClient,
				prefix:         testPrefix,
				stackDelimiter: aws.String(testDelimiter),
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
