// Opt-in AWS integration test (Floci): store-output hooks inherit the run's default identity.
package tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestAWSStoreHooks_InheritedIdentity_FlociE2E proves the hook-path default-identity fix: the stores
// declare NO `identity`, but a `default: true` Atmos auth identity is configured. With ambient AWS
// credentials cleared, the after-apply `store-outputs` hook must still write to SSM/ASM by inheriting
// the run's auto-detected identity (the same one the apply ran as).
//
// Before the fix the hook path was resolver-only — identity-less stores fell back to the default AWS
// SDK credential chain (empty under Atmos auth) and failed with "no EC2 IMDS role found". This test
// fails on the old behavior and passes with the fix.
func TestAWSStoreHooks_InheritedIdentity_FlociE2E(t *testing.T) {
	RequireTofu(t)

	harness := newFlociHarness(t, flociHarnessOptions{
		FixtureDir:  "fixtures/scenarios/aws-store-hooks-floci-inherit",
		ClearAWSEnv: true,
		ExtraEnv: map[string]string{
			"FLOCI_STORE_HOOKS_SSM_PREFIX": "/atmos-tests/store-hooks-inherit/{test_id}",
			"FLOCI_STORE_HOOKS_ASM_PREFIX": "atmos-tests/store-hooks-inherit/{test_id}",
		},
	})

	ssmClient := harness.SSMClient(t)
	asmClient := harness.SecretsManagerClient(t)
	ssmPrefix := harness.Env["FLOCI_STORE_HOOKS_SSM_PREFIX"]
	asmPrefix := harness.Env["FLOCI_STORE_HOOKS_ASM_PREFIX"]

	defer cleanupFlociSSMParameters(t, ssmClient, []string{
		ssmPrefix + "/producer/output-demo/demo_id",
	})
	defer cleanupFlociSecrets(t, asmClient, []string{
		asmPrefix + "/producer/output-demo/demo_id",
	})

	_, stderr, err := harness.Run(t, 2*time.Minute, "validate", "stacks")
	require.NoError(t, err, "validate stacks failed:\n%s", stderr)

	// Apply: terraform succeeds, then the after-apply store-output hook must succeed by inheriting the
	// default identity (no per-store `identity` configured, ambient AWS creds cleared).
	_, stderr, err = harness.Run(
		t, 3*time.Minute,
		"terraform", "apply", "output-demo", "-s", "producer", "-auto-approve",
	)
	require.NoError(t, err, "apply with inherited store identity should succeed:\n%s", stderr)

	requireFlociParameterValueContains(t, ssmClient, ssmPrefix+"/producer/output-demo/demo_id", "store-hooks-demo")
	requireFlociSecretValueContains(t, asmClient, asmPrefix+"/producer/output-demo/demo_id", "store-hooks-demo")
}
