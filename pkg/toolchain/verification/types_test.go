package verification

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

var _ = schema.ToolchainVerification{
	Checksums:       "",
	Signatures:      "",
	VerifierInstall: "",
}

func TestPolicyFromConfigDefaults(t *testing.T) {
	assert.Equal(t, Policy{
		Checksums:       PolicyWhenAvailable,
		Signatures:      PolicyWhenAvailable,
		VerifierInstall: VerifierInstallAuto,
	}, PolicyFromConfig(nil))
}

func TestPolicyFromConfigPreservesValidValues(t *testing.T) {
	policy := PolicyFromConfig(&schema.ToolchainVerification{
		Checksums:       PolicyRequired,
		Signatures:      PolicyDisabled,
		VerifierInstall: VerifierInstallPathOnly,
	})

	assert.Equal(t, PolicyRequired, policy.Checksums)
	assert.Equal(t, PolicyDisabled, policy.Signatures)
	assert.Equal(t, VerifierInstallPathOnly, policy.VerifierInstall)
}

func TestPolicyFromConfigDefaultsInvalidValues(t *testing.T) {
	policy := PolicyFromConfig(&schema.ToolchainVerification{
		Checksums:       "strict",
		Signatures:      "never",
		VerifierInstall: "manual",
	})

	assert.Equal(t, PolicyWhenAvailable, policy.Checksums)
	assert.Equal(t, PolicyWhenAvailable, policy.Signatures)
	assert.Equal(t, VerifierInstallAuto, policy.VerifierInstall)
}
