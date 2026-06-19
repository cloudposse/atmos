package store

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errTestSetAuth is a sentinel used to prove SetAuthContext clears initErr on identity change.
var errTestSetAuth = errors.New("primed init error")

func TestMarshalSecretsManagerValue(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{
			name:  "json object string passes through verbatim",
			value: `{"key":"value"}`,
			want:  `{"key":"value"}`,
		},
		{
			name:  "json array string passes through verbatim",
			value: `["a","b"]`,
			want:  `["a","b"]`,
		},
		{
			name:  "json object string with surrounding whitespace is trimmed",
			value: "  {\"key\":\"value\"}\n",
			want:  `{"key":"value"}`,
		},
		{
			name:  "invalid json string is encoded as quoted json",
			value: `{not valid json`,
			want:  `"{not valid json"`,
		},
		{
			name:  "plain word string is encoded as quoted json",
			value: "hello",
			want:  `"hello"`,
		},
		{
			name:  "empty string is encoded as quoted json",
			value: "",
			want:  `""`,
		},
		{
			name:  "non-string value is marshaled to json",
			value: map[string]any{"k": "v"},
			want:  `{"k":"v"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := marshalSecretsManagerValue(tt.value)
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(got))
		})
	}
}

func TestNewSecretsManagerStore_EndpointFallback(t *testing.T) {
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
			endpoint:     aws.String("http://endpoint"),
			endpointURL:  aws.String("http://endpoint-url"),
			wantEndpoint: "http://endpoint",
		},
		{
			name:         "endpoint_url used when endpoint nil",
			endpointURL:  aws.String("http://endpoint-url"),
			wantEndpoint: "http://endpoint-url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewSecretsManagerStore(SecretsManagerStoreOptions{
				Region:      "us-east-1",
				Endpoint:    tt.endpoint,
				EndpointURL: tt.endpointURL,
			}, "")
			require.NoError(t, err)
			asm, ok := s.(*SecretsManagerStore)
			require.True(t, ok)
			assert.Equal(t, tt.wantEndpoint, asm.endpoint)
		})
	}
}

func TestSecretsManagerStore_InitDefaultClient_WithEndpoint(t *testing.T) {
	s := &SecretsManagerStore{region: "us-east-1", endpoint: "http://localhost:4566"}

	require.NoError(t, s.initDefaultClient())
	assert.NotNil(t, s.client)
}

func TestSecretsManagerStore_SetAuthContext_ResetOnIdentityChange(t *testing.T) {
	delim := "-"
	primeState := func(s *SecretsManagerStore) {
		s.client = newFakeSecretsManager()
		s.initErr = errTestSetAuth
		s.initOnce.Do(func() {})
	}

	t.Run("different identity resets client state", func(t *testing.T) {
		s := &SecretsManagerStore{
			identityName:   "aws/old",
			region:         "us-east-1",
			stackDelimiter: &delim,
		}
		primeState(s)

		s.SetAuthContext(nil, "aws/new")

		assert.Equal(t, "aws/new", s.identityName)
		assert.Nil(t, s.client)
		assert.NoError(t, s.initErr)
	})

	t.Run("same identity preserves client state", func(t *testing.T) {
		s := &SecretsManagerStore{
			identityName:   "aws/same",
			region:         "us-east-1",
			stackDelimiter: &delim,
		}
		primeState(s)

		s.SetAuthContext(nil, "aws/same")

		assert.Equal(t, "aws/same", s.identityName)
		assert.NotNil(t, s.client)
		assert.ErrorIs(t, s.initErr, errTestSetAuth)
	})

	t.Run("empty identity preserves client state", func(t *testing.T) {
		s := &SecretsManagerStore{
			identityName:   "aws/keep",
			region:         "us-east-1",
			stackDelimiter: &delim,
		}
		primeState(s)

		s.SetAuthContext(nil, "")

		assert.Equal(t, "aws/keep", s.identityName)
		assert.NotNil(t, s.client)
		assert.ErrorIs(t, s.initErr, errTestSetAuth)
	})
}
