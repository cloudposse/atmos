package target

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	emu "github.com/cloudposse/atmos/pkg/emulator"
)

func awsEndpoint() *emu.Endpoint {
	return &emu.Endpoint{Target: emu.TargetAWS, Host: "localhost", Ports: map[int]int{4566: 4566}}
}

func TestAWSProfile(t *testing.T) {
	ep := &emu.Endpoint{Target: emu.TargetAWS, Host: "localhost", Ports: map[int]int{4566: 54321}, Region: "eu-west-1"}
	p := AWSProfile(ep)
	assert.Equal(t, "http://127.0.0.1:54321", p.Env["AWS_ENDPOINT_URL"])
	assert.Equal(t, "http://127.0.0.1:54321", p.Env["AWS_ENDPOINT_URL_S3"], "per-service S3 endpoint for the state backend")
	assert.Equal(t, "test", p.Env["AWS_ACCESS_KEY_ID"])
	assert.Equal(t, "eu-west-1", p.Env["AWS_REGION"])
	assert.Equal(t, true, p.Provider["s3_use_path_style"])
}

func TestAWSProfile_DefaultRegion(t *testing.T) {
	p := AWSProfile(awsEndpoint())
	assert.Equal(t, awsDefaultRegion, p.Env["AWS_REGION"])
}

func TestGCPProfile(t *testing.T) {
	ep := &emu.Endpoint{Target: emu.TargetGCP, Host: "localhost", Ports: map[int]int{4588: 30001}, Project: "demo"}
	p := GCPProfile(ep)
	assert.Equal(t, "127.0.0.1:30001", p.Env["PUBSUB_EMULATOR_HOST"])
	assert.Equal(t, "http://127.0.0.1:30001", p.Env["STORAGE_EMULATOR_HOST"])
	assert.Equal(t, "demo", p.Env["GOOGLE_CLOUD_PROJECT"])
	assert.Equal(t, "true", p.Env["CLOUDSDK_AUTH_DISABLE_CREDENTIALS"])
}

func TestAzureProfile(t *testing.T) {
	ep := &emu.Endpoint{Target: emu.TargetAzure, Host: "localhost", Ports: map[int]int{4577: 30002}}
	p := AzureProfile(ep)
	assert.Equal(t, azuriteAccount, p.Env["AZURE_STORAGE_ACCOUNT"])
	assert.True(t, strings.Contains(p.Env["AZURE_STORAGE_CONNECTION_STRING"], "BlobEndpoint=http://127.0.0.1:30002/devstoreaccount1"))
}

func TestVaultProfile(t *testing.T) {
	ep := &emu.Endpoint{Target: emu.TargetVault, Host: "localhost", Ports: map[int]int{8200: 8200}}
	p := VaultProfile(ep)
	assert.Equal(t, "http://127.0.0.1:8200", p.Env["VAULT_ADDR"])
	assert.Equal(t, "http://127.0.0.1:8200", p.Env["BAO_ADDR"])
	// The root token is dynamic (file-backed server); the manager harvests it in
	// Resolve, so the profile builder does not set it.
	assert.NotContains(t, p.Env, "VAULT_TOKEN")
}

func TestRegistryProfile(t *testing.T) {
	ep := &emu.Endpoint{Target: emu.TargetRegistry, Host: "localhost", Ports: map[int]int{5000: 35000}}
	p := RegistryProfile(ep)
	assert.Equal(t, "127.0.0.1:35000", p.Env["ATMOS_REGISTRY_HOST"])
}

func TestKubernetesProfile_Empty(t *testing.T) {
	// The kubeconfig is harvested by the identity, not the driver.
	p := KubernetesProfile(awsEndpoint())
	assert.Empty(t, p.Env)
	assert.Nil(t, p.Kubeconfig)
}
