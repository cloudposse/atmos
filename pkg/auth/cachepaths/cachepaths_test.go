package cachepaths

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAll(t *testing.T) {
	assert.Equal(t, []string{AWSSSOSubdir, AzureDeviceCodeSubdir, AWSWebflowSubdir, ProvisioningSubdir}, All())
}
