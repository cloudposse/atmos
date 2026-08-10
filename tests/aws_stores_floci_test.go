//nolint:depguard // This opt-in AWS integration test verifies AWS-compatible Floci stores.
package tests

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	secretsmanagertypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/stretchr/testify/require"
)

func TestAWSStoreHooksFlociE2E(t *testing.T) {
	RequireTofu(t)

	harness := newFlociHarness(t, flociHarnessOptions{
		FixtureDir:  "fixtures/scenarios/aws-store-hooks-floci",
		ClearAWSEnv: true,
		ExtraEnv: map[string]string{
			"FLOCI_STORE_HOOKS_SSM_PREFIX": "/atmos-tests/store-hooks/{test_id}",
			"FLOCI_STORE_HOOKS_ASM_PREFIX": "atmos-tests/store-hooks/{test_id}",
		},
	})

	ssmClient := harness.SSMClient(t)
	asmClient := harness.SecretsManagerClient(t)
	ssmPrefix := harness.Env["FLOCI_STORE_HOOKS_SSM_PREFIX"]
	asmPrefix := harness.Env["FLOCI_STORE_HOOKS_ASM_PREFIX"]

	defer cleanupFlociSSMParameters(t, ssmClient, []string{
		ssmPrefix + "/producer/output-demo/demo_id",
		ssmPrefix + "/producer/output-demo/structured_config",
		ssmPrefix + "/producer/output-demo/secret_like_value",
	})
	defer cleanupFlociSecrets(t, asmClient, []string{
		asmPrefix + "/producer/output-demo/demo_id",
		asmPrefix + "/producer/output-demo/structured_config",
		asmPrefix + "/producer/output-demo/secret_like_value",
	})

	_, stderr, err := harness.Run(t, 2*time.Minute, "validate", "stacks")
	require.NoError(t, err, "validate stacks failed:\n%s", stderr)

	_, stderr, err = harness.Run(
		t, 3*time.Minute,
		"terraform", "apply", "output-demo", "-s", "producer", "-auto-approve",
	)
	require.NoError(t, err, "producer apply failed:\n%s", stderr)

	requireFlociParameterValueContains(t, ssmClient, ssmPrefix+"/producer/output-demo/demo_id", "store-hooks-demo")
	requireFlociSecretValueContains(t, asmClient, asmPrefix+"/producer/output-demo/demo_id", "store-hooks-demo")

	producerDescribe, stderr, err := harness.Run(
		t, 2*time.Minute,
		"describe", "component", "reader", "-s", "producer", "--format", "json",
	)
	require.NoError(t, err, "producer reader describe failed:\n%s", stderr)
	require.Contains(t, producerDescribe, "store-hooks-demo")
	require.Contains(t, producerDescribe, "fallback-value")

	consumerDescribe, stderr, err := harness.Run(
		t, 2*time.Minute,
		"describe", "component", "reader", "-s", "consumer", "--format", "json",
	)
	require.NoError(t, err, "consumer reader describe failed:\n%s", stderr)
	require.Contains(t, consumerDescribe, "store-hooks-demo")
	require.Contains(t, consumerDescribe, "https://store-hooks.example.com")
}

func TestAWSSecretsFlociE2E(t *testing.T) {
	RequireTofu(t)

	harness := newFlociHarness(t, flociHarnessOptions{
		FixtureDir:  "fixtures/scenarios/aws-secrets-floci",
		ClearAWSEnv: true,
		ExtraEnv: map[string]string{
			"FLOCI_SECRETS_SSM_PREFIX": "/atmos-tests/secrets/{test_id}",
			"FLOCI_SECRETS_ASM_PREFIX": "atmos-tests/secrets/{test_id}",
		},
	})

	ssmClient := harness.SSMClient(t)
	asmClient := harness.SecretsManagerClient(t)
	ssmPrefix := harness.Env["FLOCI_SECRETS_SSM_PREFIX"]
	asmPrefix := harness.Env["FLOCI_SECRETS_ASM_PREFIX"]

	defer cleanupFlociSSMParameters(t, ssmClient, []string{
		ssmPrefix + "/dev/secret-consumer/SSM_INSTANCE_TOKEN",
		ssmPrefix + "/dev/SSM_STACK_TOKEN",
		ssmPrefix + "/GLOBAL_SHARED_TOKEN",
	})
	defer cleanupFlociSecrets(t, asmClient, []string{
		asmPrefix + "/dev/secret-consumer/ASM_DATABASE_CONFIG",
	})

	_, stderr, err := harness.Run(t, 2*time.Minute, "validate", "stacks")
	require.NoError(t, err, "validate stacks failed:\n%s", stderr)

	setSecret := func(target string, args ...string) {
		t.Helper()
		cmdArgs := []string{"secret", "set", target, "-s", "dev", "--force"}
		cmdArgs = append(cmdArgs, args...)
		_, setStderr, setErr := harness.Run(t, 2*time.Minute, cmdArgs...)
		require.NoError(t, setErr, "secret set %s failed:\n%s", target, setStderr)
	}
	setSecret("SSM_INSTANCE_TOKEN=dev-instance-token", "-c", "secret-consumer")
	setSecret("SSM_STACK_TOKEN=dev-stack-token", "-c", "secret-consumer")
	setSecret(`ASM_DATABASE_CONFIG={"username":"demo","password":"dev-db-password","host":"db.dev.local"}`, "-c", "secret-consumer")
	setSecret("GLOBAL_SHARED_TOKEN=shared-token", "-c", "secret-consumer")

	requireFlociParameterValueContains(t, ssmClient, ssmPrefix+"/dev/secret-consumer/SSM_INSTANCE_TOKEN", "dev-instance-token")
	requireFlociParameterValueContains(t, ssmClient, ssmPrefix+"/dev/SSM_STACK_TOKEN", "dev-stack-token")
	requireFlociParameterValueContains(t, ssmClient, ssmPrefix+"/GLOBAL_SHARED_TOKEN", "shared-token")
	requireFlociSecretValueContains(t, asmClient, asmPrefix+"/dev/secret-consumer/ASM_DATABASE_CONFIG", "dev-db-password")

	listOut, stderr, err := harness.Run(t, 2*time.Minute, "secret", "list", "-s", "dev", "-c", "secret-consumer")
	require.NoError(t, err, "secret list failed:\n%s", stderr)
	require.Contains(t, listOut, "SSM_INSTANCE_TOKEN")
	require.Contains(t, listOut, "ASM_DATABASE_CONFIG")

	_, stderr, err = harness.Run(t, 2*time.Minute, "secret", "validate", "-s", "dev", "-c", "secret-consumer")
	require.NoError(t, err, "secret validate failed:\n%s", stderr)

	passwordOut, stderr, err := harness.Run(
		t, 2*time.Minute,
		"secret", "get", "ASM_DATABASE_CONFIG", "-s", "dev", "-c", "secret-consumer", "--path", ".password", "--mask=false",
	)
	require.NoError(t, err, "secret get failed:\n%s", stderr)
	require.Contains(t, passwordOut, "dev-db-password")

	maskedDescribe, stderr, err := harness.Run(
		t, 2*time.Minute,
		"describe", "component", "secret-consumer", "-s", "dev", "--format", "json",
	)
	require.NoError(t, err, "masked describe failed:\n%s", stderr)
	require.Contains(t, maskedDescribe, "<MASKED>")
	require.NotContains(t, maskedDescribe, "dev-instance-token")
	require.NotContains(t, maskedDescribe, "dev-db-password")

	_, stderr, err = harness.Run(
		t, 3*time.Minute,
		"terraform", "apply", "secret-consumer", "-s", "dev", "-auto-approve",
	)
	require.NoError(t, err, "secret-consumer apply failed:\n%s", stderr)
}

func requireFlociParameterValueContains(t *testing.T, client *ssm.Client, name, expected string) {
	t.Helper()

	require.Eventually(t, func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		out, err := client.GetParameter(ctx, &ssm.GetParameterInput{
			Name:           aws.String(name),
			WithDecryption: aws.Bool(true),
		})
		if err != nil {
			t.Logf("checking Floci parameter %s: %v", name, err)
			return false
		}
		return out.Parameter != nil && out.Parameter.Value != nil && strings.Contains(*out.Parameter.Value, expected)
	}, 10*time.Second, 200*time.Millisecond, "expected Floci parameter %s to contain %q", name, expected)
}

func requireFlociSecretValueContains(t *testing.T, client *secretsmanager.Client, secretID, expected string) {
	t.Helper()

	require.Eventually(t, func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		out, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
			SecretId: aws.String(secretID),
		})
		if err != nil {
			t.Logf("checking Floci secret %s: %v", secretID, err)
			return false
		}
		return out.SecretString != nil && strings.Contains(*out.SecretString, expected)
	}, 10*time.Second, 200*time.Millisecond, "expected Floci secret %s to contain %q", secretID, expected)
}

func cleanupFlociSSMParameters(t *testing.T, client *ssm.Client, names []string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := client.DeleteParameters(ctx, &ssm.DeleteParametersInput{Names: names})
	if err != nil {
		var notFound *ssmtypes.ParameterNotFound
		if !errors.As(err, &notFound) {
			t.Logf("best-effort Floci SSM cleanup failed: %v", err)
		}
	}
}

func cleanupFlociSecrets(t *testing.T, client *secretsmanager.Client, secretIDs []string) {
	t.Helper()

	for _, secretID := range secretIDs {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := client.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{
			SecretId:                   aws.String(secretID),
			ForceDeleteWithoutRecovery: aws.Bool(true),
		})
		cancel()

		var notFound *secretsmanagertypes.ResourceNotFoundException
		if err != nil && !errors.As(err, &notFound) {
			t.Logf("best-effort Floci Secrets Manager cleanup failed for %s: %v", secretID, err)
		}
	}
}
