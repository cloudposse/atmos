package aws

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAWSCommandProvider_GetCommand(t *testing.T) {
	provider := &AWSCommandProvider{}
	cmd := provider.GetCommand()

	assert.NotNil(t, cmd)
	assert.Equal(t, "aws", cmd.Use)
	assert.Contains(t, cmd.Short, "AWS")
}

func TestAWSCommandProvider_GetName(t *testing.T) {
	provider := &AWSCommandProvider{}
	name := provider.GetName()

	assert.Equal(t, "aws", name)
}

func TestAWSCommandProvider_GetGroup(t *testing.T) {
	provider := &AWSCommandProvider{}
	group := provider.GetGroup()

	assert.Equal(t, "Cloud Integration", group)
}

func TestAWSCommandProvider_GetAliases(t *testing.T) {
	provider := &AWSCommandProvider{}
	aliases := provider.GetAliases()

	assert.Nil(t, aliases)
}

func TestAWSCommandProvider_GetFlagsBuilder(t *testing.T) {
	provider := &AWSCommandProvider{}
	builder := provider.GetFlagsBuilder()

	assert.Nil(t, builder)
}

func TestAWSCommandProvider_GetPositionalArgsBuilder(t *testing.T) {
	provider := &AWSCommandProvider{}
	builder := provider.GetPositionalArgsBuilder()

	assert.Nil(t, builder)
}

func TestAWSCommandProvider_GetCompatibilityFlags(t *testing.T) {
	provider := &AWSCommandProvider{}
	flags := provider.GetCompatibilityFlags()

	assert.Nil(t, flags)
}

func TestAWSCommand_HasEksSubcommand(t *testing.T) {
	provider := &AWSCommandProvider{}
	cmd := provider.GetCommand()

	// Find the EKS subcommand.
	var foundEks bool
	for _, subCmd := range cmd.Commands() {
		if subCmd.Use == "eks" {
			foundEks = true
			break
		}
	}

	assert.True(t, foundEks, "aws command should have eks subcommand")
}
