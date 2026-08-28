package backend

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// applyS3Options applies a slice of functional options to a fresh s3.Options and
// returns the result, for asserting which options s3ClientOptions produced.
func applyS3Options(opts []func(*s3.Options)) s3.Options {
	var o s3.Options
	for _, fn := range opts {
		fn(&o)
	}
	return o
}

func TestS3ClientOptions(t *testing.T) {
	tests := []struct {
		name             string
		config           *s3Config
		authContext      *schema.AuthContext
		wantBaseEndpoint string
		wantPathStyle    bool
	}{
		{
			name:        "nil auth context and no path-style produces no options",
			config:      &s3Config{},
			authContext: nil,
		},
		{
			name:        "auth context without AWS section produces no options",
			config:      &s3Config{},
			authContext: &schema.AuthContext{},
		},
		{
			name:   "empty endpoint URL produces no BaseEndpoint option",
			config: &s3Config{},
			authContext: &schema.AuthContext{
				AWS: &schema.AWSAuthContext{EndpointURL: ""},
			},
		},
		{
			name:   "identity endpoint URL produces BaseEndpoint option",
			config: &s3Config{},
			authContext: &schema.AuthContext{
				AWS: &schema.AWSAuthContext{EndpointURL: "http://127.0.0.1:4566"},
			},
			wantBaseEndpoint: "http://127.0.0.1:4566",
		},
		{
			name:          "usePathStyle true produces UsePathStyle option",
			config:        &s3Config{usePathStyle: true},
			authContext:   nil,
			wantPathStyle: true,
		},
		{
			name:   "identity endpoint URL and usePathStyle both produce both options",
			config: &s3Config{usePathStyle: true},
			authContext: &schema.AuthContext{
				AWS: &schema.AWSAuthContext{EndpointURL: "http://127.0.0.1:4566"},
			},
			wantBaseEndpoint: "http://127.0.0.1:4566",
			wantPathStyle:    true,
		},
		{
			name:   "explicit backend config endpoint takes priority over identity endpoint",
			config: &s3Config{endpoint: "http://localhost:9000"},
			authContext: &schema.AuthContext{
				AWS: &schema.AWSAuthContext{EndpointURL: "http://127.0.0.1:4566"},
			},
			wantBaseEndpoint: "http://localhost:9000",
		},
		{
			name:             "explicit backend config endpoint used with nil auth context",
			config:           &s3Config{endpoint: "http://localhost:9000"},
			authContext:      nil,
			wantBaseEndpoint: "http://localhost:9000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := s3ClientOptions(tt.config, tt.authContext)
			got := applyS3Options(opts)

			if tt.wantBaseEndpoint == "" {
				assert.Nil(t, got.BaseEndpoint)
			} else if assert.NotNil(t, got.BaseEndpoint) {
				assert.Equal(t, tt.wantBaseEndpoint, *got.BaseEndpoint)
			}

			assert.Equal(t, tt.wantPathStyle, got.UsePathStyle)
		})
	}
}
