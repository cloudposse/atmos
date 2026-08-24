package cloudformation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNeedsPackaging(t *testing.T) {
	assert.False(t, needsPackaging("small template"))
	assert.False(t, needsPackaging(strings.Repeat("a", templateInlineSizeLimit)))
	assert.True(t, needsPackaging(strings.Repeat("a", templateInlineSizeLimit+1)))
}

func TestPackageObjectName(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "vpc"}

	name := packageObjectName("", info, "abcdef1234567890")
	assert.Equal(t, "dev/vpc/template-abcdef123456.yaml", name)

	prefixed := packageObjectName("assets/", info, "abcdef1234567890")
	assert.Equal(t, "assets/dev/vpc/template-abcdef123456.yaml", prefixed)
}

func TestPackageURL(t *testing.T) {
	url := packageURL(&targetS3Config{Bucket: "my-bucket"}, "dev/vpc/template-abc.yaml")
	assert.Equal(t, "s3://my-bucket/dev/vpc/template-abc.yaml", url)
}
