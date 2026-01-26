package eks

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUpdateKubeconfigCmd_Error(t *testing.T) {
	err := updateKubeconfigCmd.RunE(updateKubeconfigCmd, []string{})
	assert.Error(t, err, "aws eks update-kubeconfig command should return an error when called with no parameters")
}
