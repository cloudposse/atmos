package artifact

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel: renaming the schema field the registry test sets must fail the build.
var _ = schema.ArtifactRepositoryConfig{Kind: ""}

// TestEchoedValues_DoNotEchoCredentials checks every validator and error that quotes a configured
// name or value, other than the registry, prefix, key pattern and reference covered above, when
// that value is a pasted credential-bearing URL.
func TestEchoedValues_DoNotEchoCredentials(t *testing.T) {
	const credential = "https://usr7:S3cr3tA@example.com"
	registry := NewRepositoryRegistry()
	_, registryErr := registry.New(context.Background(), credential, &schema.ArtifactRepositoryConfig{Kind: credential})
	_, nilConfigErr := registry.New(context.Background(), credential, nil)
	_, permissionsErr := ParsePermissions(credential)

	tests := []struct {
		name string
		err  error
	}{
		{name: "s3 bucket", err: (&S3Options{Bucket: credential}).Validate()},
		{name: "s3 region", err: (&S3Options{Bucket: "bucket", Region: credential}).Validate()},
		{name: "s3 reserved tag key", err: validateS3Tags(map[string]string{"aws:" + credential: "v"})},
		{name: "s3 oversized tag value", err: validateS3Tags(map[string]string{credential: strings.Repeat("v", maxS3TagValLen+1)})},
		{name: "label key", err: ValidateLabel(credential, "v")},
		{name: "label key whitespace", err: ValidateLabel(" "+credential, "v")},
		{name: "label value", err: ValidateLabel(credential, strings.Repeat("v", MaxLabelValueLength+1))},
		{name: "label set", err: ValidateLabels(map[string]string{credential: "v"})},
		{name: "permissions", err: permissionsErr},
		{name: "registry unknown kind", err: registryErr},
		{name: "registry nil configuration", err: nilConfigErr},
		{name: "capability repository", err: NewUnsupportedCapability(credential, "push", "kind/"+credential, "not available")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Error(t, tt.err)
			assert.NotContains(t, tt.err.Error(), "S3cr3tA")
			assert.NotContains(t, tt.err.Error(), "usr7")
		})
	}
}

// TestEchoedValues_KeepNamesWithoutCredentials is the negative path of the test above: a plain
// name is still echoed, because the name is what lets the user find the mistake.
func TestEchoedValues_KeepNamesWithoutCredentials(t *testing.T) {
	_, registryErr := NewRepositoryRegistry().New(context.Background(), "prod", &schema.ArtifactRepositoryConfig{Kind: "ftp/server"})
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "s3 bucket", err: (&S3Options{Bucket: "Not_A_Bucket"}).Validate(), want: `bucket "Not_A_Bucket"`},
		{name: "s3 region", err: (&S3Options{Bucket: "bucket", Region: "Not A Region"}).Validate(), want: `region "Not A Region"`},
		{name: "label key", err: ValidateLabel("a=b", "v"), want: `label key "a=b"`},
		{name: "registry unknown kind", err: registryErr, want: `repository "prod" has unknown kind "ftp/server"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Error(t, tt.err)
			assert.ErrorIs(t, tt.err, errUtils.ErrArtifactConfigInvalid)
			assert.Contains(t, tt.err.Error(), tt.want)
		})
	}
}
