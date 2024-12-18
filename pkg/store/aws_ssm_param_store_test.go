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

	tests := []struct {
		name    string
		key     string
		value   interface{}
		mockFn  func(*MockSSMClient)
		wantErr bool
	}{
		{
			name:  "successful set",
			key:   "/test/key",
			value: "test-value",
			mockFn: func(m *MockSSMClient) {
				m.On("PutParameter", mock.Anything, &ssm.PutParameterInput{
					Name:      aws.String("/test/key"),
					Value:     aws.String("test-value"),
					Type:      types.ParameterTypeString,
					Overwrite: &mockFnOverwrite,
				}).Return(&ssm.PutParameterOutput{}, nil)
			},
			wantErr: false,
		},
		{
			name:  "aws error",
			key:   "/test/key",
			value: "test-value",
			mockFn: func(m *MockSSMClient) {
				m.On("PutParameter", mock.Anything, mock.Anything).
					Return(nil, errors.New("aws error"))
			},
			wantErr: true,
		},
		{
			name:    "invalid value type",
			key:     "/test/key",
			value:   123, // Not a string
			mockFn:  func(m *MockSSMClient) {},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(MockSSMClient)
			tt.mockFn(mockClient)

			store := &SSMStore{client: mockClient}
			err := store.Set(tt.key, tt.value)

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

	tests := []struct {
		name    string
		key     string
		mockFn  func(*MockSSMClient)
		want    interface{}
		wantErr bool
	}{
		{
			name: "successful get",
			key:  "/test/key",
			mockFn: func(m *MockSSMClient) {
				m.On("GetParameter", mock.Anything, &ssm.GetParameterInput{
					Name:           aws.String("/test/key"),
					WithDecryption: &mockFnWithDecryption,
				}).Return(&ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: aws.String("test-value"),
					},
				}, nil)
			},
			want:    "test-value",
			wantErr: false,
		},
		{
			name: "aws error",
			key:  "/test/key",
			mockFn: func(m *MockSSMClient) {
				m.On("GetParameter", mock.Anything, mock.Anything).
					Return(nil, errors.New("aws error"))
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(MockSSMClient)
			tt.mockFn(mockClient)

			store := &SSMStore{client: mockClient}
			got, err := store.Get(tt.key)

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
