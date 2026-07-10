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
		authContext      *schema.AuthContext
		usePathStyle     bool
		wantBaseEndpoint string
		wantPathStyle    bool
	}{
		{
			name:         "nil auth context and no path-style produces no options",
			authContext:  nil,
			usePathStyle: false,
		},
		{
			name:         "auth context without AWS section produces no options",
			authContext:  &schema.AuthContext{},
			usePathStyle: false,
		},
		{
			name: "empty endpoint URL produces no BaseEndpoint option",
			authContext: &schema.AuthContext{
				AWS: &schema.AWSAuthContext{EndpointURL: ""},
			},
			usePathStyle: false,
		},
		{
			name: "endpoint URL set produces BaseEndpoint option",
			authContext: &schema.AuthContext{
				AWS: &schema.AWSAuthContext{EndpointURL: "http://127.0.0.1:4566"},
			},
			usePathStyle:     false,
			wantBaseEndpoint: "http://127.0.0.1:4566",
		},
		{
			name:          "usePathStyle true produces UsePathStyle option",
			authContext:   nil,
			usePathStyle:  true,
			wantPathStyle: true,
		},
		{
			name: "endpoint URL and usePathStyle both produce both options",
			authContext: &schema.AuthContext{
				AWS: &schema.AWSAuthContext{EndpointURL: "http://127.0.0.1:4566"},
			},
			usePathStyle:     true,
			wantBaseEndpoint: "http://127.0.0.1:4566",
			wantPathStyle:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := s3ClientOptions(tt.authContext, tt.usePathStyle)
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
