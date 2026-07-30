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
	VerifierTrust:   "",
}

func TestPolicyFromConfigDefaults(t *testing.T) {
	assert.Equal(t, Policy{
		Checksums:       PolicyWhenAvailable,
		Signatures:      PolicyWhenAvailable,
		VerifierInstall: VerifierInstallAuto,
		VerifierTrust:   VerifierTrustAuto,
	}, PolicyFromConfig(nil))
}

func TestPolicyFromConfigPreservesValidValues(t *testing.T) {
	policy := PolicyFromConfig(&schema.ToolchainVerification{
		Checksums:       PolicyRequired,
		Signatures:      PolicyDisabled,
		VerifierInstall: VerifierInstallPathOnly,
		VerifierTrust:   VerifierTrustDisabled,
	})

	assert.Equal(t, PolicyRequired, policy.Checksums)
	assert.Equal(t, PolicyDisabled, policy.Signatures)
	assert.Equal(t, VerifierInstallPathOnly, policy.VerifierInstall)
	assert.Equal(t, VerifierTrustDisabled, policy.VerifierTrust)
}

func TestPolicyFromConfigDefaultsInvalidValues(t *testing.T) {
	policy := PolicyFromConfig(&schema.ToolchainVerification{
		Checksums:       "strict",
		Signatures:      "never",
		VerifierInstall: "manual",
		VerifierTrust:   "sometimes",
	})

	assert.Equal(t, PolicyWhenAvailable, policy.Checksums)
	assert.Equal(t, PolicyWhenAvailable, policy.Signatures)
	assert.Equal(t, VerifierInstallAuto, policy.VerifierInstall)
	assert.Equal(t, VerifierTrustAuto, policy.VerifierTrust)
}

func TestDefaultVerifierTrust(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "auto", value: VerifierTrustAuto, want: VerifierTrustAuto},
		{name: "disabled", value: VerifierTrustDisabled, want: VerifierTrustDisabled},
		{name: "empty defaults to auto", value: "", want: VerifierTrustAuto},
		{name: "invalid defaults to auto", value: "sometimes", want: VerifierTrustAuto},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, defaultVerifierTrust(tt.value))
		})
	}
}
