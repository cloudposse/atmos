package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAwsEksCmdUpdateKubeconfigCmd_Error(t *testing.T) {
	err := awsEksCmdUpdateKubeconfigCmd.RunE(awsEksCmdUpdateKubeconfigCmd, []string{})
	assert.Error(t, err, "aws eks update-kubeconfig command should return an error when called with no parameters")
}
