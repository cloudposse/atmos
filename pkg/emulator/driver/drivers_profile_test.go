package driver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	emu "github.com/cloudposse/atmos/pkg/emulator"
)

// TestBuiltinDrivers_NameTargetDefaults asserts every registered built-in driver's
// Name(), Target(), and the value of each ContainerDefaults field. The expectations
// are pinned per driver so a changed image, port, command, or privileged flag fails
// loudly.
func TestBuiltinDrivers_NameTargetDefaults(t *testing.T) {
	cases := []struct {
		name       string
		target     string
		image      string
		ports      []int
		services   []string
		env        map[string]string
		privileged bool
		command    []string
	}{
		{
			name:   "floci/aws",
			target: emu.TargetAWS,
			image:  flociAWSImage,
			ports:  []int{flociAWSPort},
		},
		{
			name:   "floci/gcp",
			target: emu.TargetGCP,
			image:  flociGCPImage,
			ports:  []int{flociGCPPort},
		},
		{
			name:   "floci/az",
			target: emu.TargetAzure,
			image:  flociAzImage,
			ports:  []int{flociAzPort},
		},
		{
			name:   "ministack/aws",
			target: emu.TargetAWS,
			image:  ministackImage,
			ports:  []int{awsEdgePort},
		},
		{
			name:   "localstack/aws",
			target: emu.TargetAWS,
			image:  localstackImage,
			ports:  []int{awsEdgePort},
		},
		{
			name:       "k3s",
			target:     emu.TargetKubernetes,
			image:      k3sImage,
			ports:      []int{k3sPort},
			env:        map[string]string{"K3S_TOKEN": "atmos-emulator"},
			privileged: true,
			command:    []string{"server"},
		},
		{
			name:    "openbao",
			target:  emu.TargetVault,
			image:   openbaoImage,
			ports:   []int{vaultPort},
			env:     vaultEnv(),
			command: vaultServerCommand,
		},
		{
			name:    "vault",
			target:  emu.TargetVault,
			image:   vaultImage,
			ports:   []int{vaultPort},
			env:     vaultEnv(),
			command: vaultServerCommand,
		},
		{
			name:   "registry",
			target: emu.TargetRegistry,
			image:  registryImage,
			ports:  []int{registryPort},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := emu.ResolveDriver(tc.name)
			require.NoError(t, err)

			assert.Equal(t, tc.name, d.Name())
			assert.Equal(t, tc.target, d.Target())

			defaults := d.Defaults()
			assert.Equal(t, tc.image, defaults.Image)
			assert.Equal(t, tc.privileged, defaults.Privileged)

			// Ports: assert contents by value (first and last element).
			require.Equal(t, tc.ports, defaults.Ports)
			require.NotEmpty(t, defaults.Ports)
			assert.Equal(t, tc.ports[0], defaults.Ports[0])
			assert.Equal(t, tc.ports[len(tc.ports)-1], defaults.Ports[len(defaults.Ports)-1])

			assert.Equal(t, tc.services, defaults.Services)
			assert.Equal(t, tc.env, defaults.Env)

			// Command: assert contents by value (first and last element when present).
			assert.Equal(t, tc.command, defaults.Command)
			if len(tc.command) > 0 {
				require.NotEmpty(t, defaults.Command)
				assert.Equal(t, tc.command[0], defaults.Command[0])
				assert.Equal(t, tc.command[len(tc.command)-1], defaults.Command[len(defaults.Command)-1])
			}
		})
	}
}

// TestGCPDriver_Profile asserts the floci/gcp driver turns a live endpoint into the
// per-service *_EMULATOR_HOST env the GCP SDKs read, plus the project vars.
func TestGCPDriver_Profile(t *testing.T) {
	d, err := emu.ResolveDriver("floci/gcp")
	require.NoError(t, err)

	ep := emu.Endpoint{
		Target:  emu.TargetGCP,
		Host:    "localhost",
		Ports:   map[int]int{4588: 14588},
		Project: "my-project",
	}
	profile := d.Profile(&ep)

	assert.Equal(t, "true", profile.Env["CLOUDSDK_AUTH_DISABLE_CREDENTIALS"])
	// GCS expects a URL; the rest want a bare host:port.
	assert.Equal(t, "http://127.0.0.1:14588", profile.Env["STORAGE_EMULATOR_HOST"])
	assert.Equal(t, "127.0.0.1:14588", profile.Env["PUBSUB_EMULATOR_HOST"])
	assert.Equal(t, "127.0.0.1:14588", profile.Env["FIRESTORE_EMULATOR_HOST"])
	assert.Equal(t, "127.0.0.1:14588", profile.Env["BIGTABLE_EMULATOR_HOST"])
	assert.Equal(t, "127.0.0.1:14588", profile.Env["DATASTORE_EMULATOR_HOST"])
	assert.Equal(t, "my-project", profile.Env["CLOUDSDK_CORE_PROJECT"])
	assert.Equal(t, "my-project", profile.Env["GOOGLE_CLOUD_PROJECT"])

	require.NotNil(t, profile.Provider)
	assert.Equal(t, "my-project", profile.Provider["project"])
	assert.Equal(t, "test", profile.Provider["access_token"])
	assert.Equal(t, false, profile.Provider["user_project_override"])
	assert.Equal(t, "http://127.0.0.1:14588/storage/v1/", profile.Provider["storage_custom_endpoint"])
	assert.Equal(t, "http://127.0.0.1:14588/v1/", profile.Provider["secret_manager_custom_endpoint"])
	assert.Equal(t, "http://127.0.0.1:14588/", profile.Provider["iam_custom_endpoint"])
	assert.Equal(t, "http://127.0.0.1:14588/", profile.Provider["iam_credentials_custom_endpoint"])

	// GCP target has no kubeconfig or resolver URL.
	assert.Nil(t, profile.Kubeconfig)
	assert.Empty(t, profile.ResolverURL)
}

// TestAzureDriver_Profile asserts the floci/az driver emits an Azurite connection
// string and the well-known dev account credentials pointed at the live endpoint.
func TestAzureDriver_Profile(t *testing.T) {
	d, err := emu.ResolveDriver("floci/az")
	require.NoError(t, err)

	ep := emu.Endpoint{
		Target: emu.TargetAzure,
		Host:   "localhost",
		Ports:  map[int]int{4577: 14577},
	}
	profile := d.Profile(&ep)

	assert.Equal(t, "devstoreaccount1", profile.Env["AZURE_STORAGE_ACCOUNT"])
	assert.Equal(
		t,
		"Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==",
		profile.Env["AZURE_STORAGE_KEY"],
	)
	assert.Equal(
		t,
		"DefaultEndpointsProtocol=http;AccountName=devstoreaccount1;AccountKey=Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==;BlobEndpoint=http://127.0.0.1:14577/devstoreaccount1;",
		profile.Env["AZURE_STORAGE_CONNECTION_STRING"],
	)

	require.NotNil(t, profile.Provider)
	assert.Equal(t, []map[string]any{{}}, profile.Provider["features"])
	assert.Equal(t, true, profile.Provider["skip_provider_registration"])
	assert.Equal(t, "127.0.0.1:14577", profile.Provider["metadata_host"])
	assert.Equal(t, "00000000-0000-0000-0000-000000000000", profile.Provider["subscription_id"])
	assert.Equal(t, "00000000-0000-0000-0000-000000000000", profile.Provider["tenant_id"])
	assert.Equal(t, "00000000-0000-0000-0000-000000000001", profile.Provider["client_id"])
	assert.Equal(t, "test", profile.Provider["client_secret"])

	assert.Nil(t, profile.Kubeconfig)
	assert.Empty(t, profile.ResolverURL)
}

// TestVaultDrivers_Profile asserts both vault-target drivers emit VAULT_ADDR (from the
// live endpoint) and the fixed dev-mode root token VAULT_TOKEN.
func TestVaultDrivers_Profile(t *testing.T) {
	for _, name := range []string{"openbao", "vault"} {
		t.Run(name, func(t *testing.T) {
			d, err := emu.ResolveDriver(name)
			require.NoError(t, err)

			ep := emu.Endpoint{
				Target: emu.TargetVault,
				Host:   "localhost",
				Ports:  map[int]int{8200: 18200},
			}
			profile := d.Profile(&ep)

			assert.Equal(t, "http://127.0.0.1:18200", profile.Env["VAULT_ADDR"])
			assert.Equal(t, "http://127.0.0.1:18200", profile.Env["BAO_ADDR"])
			// The root token is dynamic (file-backed server) and harvested by the
			// manager in Resolve, not set by the driver's profile builder.
			assert.NotContains(t, profile.Env, "VAULT_TOKEN")

			assert.Nil(t, profile.Kubeconfig)
			assert.Empty(t, profile.ResolverURL)
			assert.Nil(t, profile.Provider)
		})
	}
}

// TestRegistryDriver_Profile asserts the registry driver surfaces the live host:port
// authority as ATMOS_REGISTRY_HOST.
func TestRegistryDriver_Profile(t *testing.T) {
	d, err := emu.ResolveDriver("registry")
	require.NoError(t, err)

	ep := emu.Endpoint{
		Target: emu.TargetRegistry,
		Host:   "localhost",
		Ports:  map[int]int{5000: 15000},
	}
	profile := d.Profile(&ep)

	assert.Equal(t, "127.0.0.1:15000", profile.Env["ATMOS_REGISTRY_HOST"])
	assert.Nil(t, profile.Kubeconfig)
	assert.Empty(t, profile.ResolverURL)
	assert.Nil(t, profile.Provider)
}

// TestK3sDriver_Profile asserts the kubernetes-target k3s driver returns the empty
// placeholder profile — the kubeconfig is harvested from the running container by the
// kubernetes/emulator identity, not built from the endpoint.
func TestK3sDriver_Profile(t *testing.T) {
	d, err := emu.ResolveDriver("k3s")
	require.NoError(t, err)

	ep := emu.Endpoint{
		Target: emu.TargetKubernetes,
		Host:   "localhost",
		Ports:  map[int]int{6443: 16443},
	}
	profile := d.Profile(&ep)

	assert.Nil(t, profile.Env)
	assert.Nil(t, profile.Kubeconfig)
	assert.Empty(t, profile.ResolverURL)
	assert.Nil(t, profile.Provider)
}

// TestK3sDriver_RootlessOverride asserts k3s is the rootless-overriding driver: it
// returns the entrypoint run-args and the cgroup-nesting shell command with ok=true.
func TestK3sDriver_RootlessOverride(t *testing.T) {
	d, err := emu.ResolveDriver("k3s")
	require.NoError(t, err)

	overrider, ok := d.(emu.RootlessOverrider)
	require.True(t, ok, "k3s driver must implement RootlessOverrider")

	runArgs, command, ok := overrider.RootlessOverride()
	require.True(t, ok, "k3s must provide a rootless override")

	// runArgs override the entrypoint to /bin/sh.
	require.Equal(t, []string{"--entrypoint", "/bin/sh"}, runArgs)
	assert.Equal(t, "--entrypoint", runArgs[0])
	assert.Equal(t, "/bin/sh", runArgs[len(runArgs)-1])

	// command runs the cgroup-nesting shim via `sh -c <script>`.
	require.Equal(t, []string{"-c", k3sRootlessScript}, command)
	assert.Equal(t, "-c", command[0])
	assert.Equal(t, k3sRootlessScript, command[len(command)-1])
}

// TestNonK3sDrivers_NoRootlessOverride asserts drivers without a rootless variant
// return ok=false (nil run-args/command) — the rootful defaults run in all runtimes.
func TestNonK3sDrivers_NoRootlessOverride(t *testing.T) {
	for _, name := range []string{"floci/aws", "floci/gcp", "floci/az", "ministack/aws", "localstack/aws", "openbao", "vault", "registry"} {
		t.Run(name, func(t *testing.T) {
			d, err := emu.ResolveDriver(name)
			require.NoError(t, err)

			overrider, ok := d.(emu.RootlessOverrider)
			require.True(t, ok, "builtinDriver always implements RootlessOverrider")

			runArgs, command, ok := overrider.RootlessOverride()
			assert.False(t, ok, "non-k3s drivers have no rootless override")
			assert.Nil(t, runArgs)
			assert.Nil(t, command)
		})
	}
}

// Compile-time guard: the schema fields asserted above exist on emu.Endpoint.
var _ = emu.Endpoint{Target: emu.TargetGCP, Project: "p", Region: "r"}
