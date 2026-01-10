package helmfile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestResolveAWSAuth(t *testing.T) {
	defaultContext := &Context{
		Namespace:   "test-ns",
		Tenant:      "tenant1",
		Environment: "dev",
		Stage:       "ue2",
		Region:      "us-east-2",
	}
	emptyContext := &Context{}

	tests := []struct {
		name                    string
		input                   AuthInput
		context                 *Context
		expectedUseIdentityAuth bool
		expectedProfile         string
		expectedSource          string
		expectedDeprecated      bool
		expectedError           error
	}{
		{
			name: "identity takes highest precedence",
			input: AuthInput{
				Identity:       "prod-admin",
				ProfilePattern: "cp-{namespace}-{stage}-helm",
			},
			context:                 defaultContext,
			expectedUseIdentityAuth: true,
			expectedProfile:         "",
			expectedSource:          "identity",
			expectedDeprecated:      false,
			expectedError:           nil,
		},
		{
			name: "pattern fallback is deprecated",
			input: AuthInput{
				Identity:       "",
				ProfilePattern: "cp-{namespace}-{stage}-helm",
			},
			context:                 defaultContext,
			expectedUseIdentityAuth: false,
			expectedProfile:         "cp-test-ns-ue2-helm",
			expectedSource:          "pattern",
			expectedDeprecated:      true,
			expectedError:           nil,
		},
		{
			name: "error when no source configured",
			input: AuthInput{
				Identity:       "",
				ProfilePattern: "",
			},
			context:                 defaultContext,
			expectedUseIdentityAuth: false,
			expectedProfile:         "",
			expectedSource:          "",
			expectedDeprecated:      false,
			expectedError:           errUtils.ErrMissingHelmfileAuth,
		},
		{
			name: "identity only - minimal input",
			input: AuthInput{
				Identity: "my-identity",
			},
			context:                 emptyContext,
			expectedUseIdentityAuth: true,
			expectedProfile:         "",
			expectedSource:          "identity",
			expectedDeprecated:      false,
			expectedError:           nil,
		},
		{
			name: "complex pattern with all tokens",
			input: AuthInput{
				ProfilePattern: "cp-{namespace}-{tenant}-gbl-{stage}-helm",
			},
			context: &Context{
				Namespace: "acme",
				Tenant:    "platform",
				Stage:     "uw2",
			},
			expectedUseIdentityAuth: false,
			expectedProfile:         "cp-acme-platform-gbl-uw2-helm",
			expectedSource:          "pattern",
			expectedDeprecated:      true,
			expectedError:           nil,
		},
		{
			name: "identity with path-style name",
			input: AuthInput{
				Identity: "core-identity/managers",
			},
			context:                 defaultContext,
			expectedUseIdentityAuth: true,
			expectedProfile:         "",
			expectedSource:          "identity",
			expectedDeprecated:      false,
			expectedError:           nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ResolveAWSAuth(tt.input, tt.context)

			if tt.expectedError != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedError)
				assert.Nil(t, result)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.expectedUseIdentityAuth, result.UseIdentityAuth)
			assert.Equal(t, tt.expectedProfile, result.Profile)
			assert.Equal(t, tt.expectedSource, result.Source)
			assert.Equal(t, tt.expectedDeprecated, result.IsDeprecated)
		})
	}
}

func BenchmarkResolveAWSAuth(b *testing.B) {
	input := AuthInput{
		Identity: "benchmark-identity",
	}
	context := &Context{}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = ResolveAWSAuth(input, context)
	}
}
