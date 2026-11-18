package downloader

import (
	"context"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/broker"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel: the injection tests configure tokens through these fields.
var _ = schema.AtmosSettings{InjectGithubToken: true, GithubToken: "x"}

// testIdentity is the identity the fetches in these tests run as.
const testIdentity = "ops"

// identityContext returns a context whose fetches run as testIdentity, with env as its environment.
func identityContext(t *testing.T, env []string) context.Context {
	t.Helper()
	registerTestResolver(t, func(context.Context, *schema.AtmosConfiguration, string) ([]string, error) {
		return env, nil
	})
	ctx, err := WithIdentity(context.Background(), &schema.AtmosConfiguration{}, testIdentity, "")
	require.NoError(t, err)
	return ctx
}

// brokerExport runs a fake credential broker that exports env into the process environment, the
// way the Atmos Pro broker does on the first ambient remote read.
func brokerExport(t *testing.T, env map[string]string) {
	t.Helper()
	for key := range env {
		// Registers the restore of whatever the broker is about to set, leaving the variable in
		// the state the test started with (unset stays unset).
		value, wasSet := os.LookupEnv(key)
		t.Setenv(key, value)
		if !wasSet {
			require.NoError(t, os.Unsetenv(key))
		}
	}
	t.Cleanup(broker.SwapRegistryForTest())
	broker.Register(fakeBroker{env: env})
	broker.EnsureCredentials(context.Background(), &schema.AtmosConfiguration{})
	for key, value := range env {
		require.Equal(t, value, os.Getenv(key), "prerequisite: the broker exported %s into the process", key)
	}
}

type fakeBroker struct{ env map[string]string }

func (fakeBroker) Name() string                            { return "fake" }
func (fakeBroker) Enabled(*schema.AtmosConfiguration) bool { return true }
func (b fakeBroker) Provision(context.Context, *schema.AtmosConfiguration) (map[string]string, error) {
	return b.env, nil
}

func TestSourceUsesIdentity(t *testing.T) {
	tests := []struct {
		src  string
		want bool
	}{
		{src: "git::https://github.com/acme/modules.git", want: true},
		{src: "github.com/acme/modules//vpc", want: true},
		{src: "git@github.com:acme/modules.git", want: true},
		{src: "ssh://git@git.example.com/acme/modules.git", want: true},
		{src: "https://example.com/stacks/catalog.yaml", want: true},
		{src: "git::https://bucket.s3.amazonaws.com/repo.git", want: true},
		{src: "oci://ghcr.io/acme/stacks:v1//catalog/vpc.yaml", want: false},
		{src: "OCI://ghcr.io/acme/stacks:v1", want: false},
		{src: "s3::https://s3.amazonaws.com/bucket/module.zip", want: false},
		{src: "s3://bucket/module.zip", want: false},
		{src: "bucket.s3.amazonaws.com/module.zip", want: false},
		{src: "gcs::https://www.googleapis.com/storage/v1/bucket/module.zip", want: false},
		{src: "www.googleapis.com/storage/v1/bucket/module.zip", want: false},
		{src: "file:///tmp/module", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			assert.Equal(t, tt.want, SourceUsesIdentity(tt.src))
		})
	}
}

func TestIsGitSource(t *testing.T) {
	tests := []struct {
		src  string
		want bool
	}{
		{src: "git::https://github.com/acme/modules.git", want: true},
		{src: "github.com/acme/modules//vpc", want: true},
		{src: "git@github.com:acme/modules.git", want: true},
		{src: "ssh://git@git.example.com/acme/modules.git", want: true},
		{src: "https://github.com/acme/modules/archive/main.tar.gz", want: false},
		{src: "https://example.com/stacks/catalog.yaml", want: false},
		{src: "./local/module", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			assert.Equal(t, tt.want, isGitSource(tt.src))
		})
	}
}

// An identity that cannot be resolved must not stop a source it would never have authenticated.
func TestWithIdentityForSource_ResolvesOnlyForTransportsThatUseIt(t *testing.T) {
	notLoggedIn := errors.New("no session")
	var calls atomic.Int32
	registerTestResolver(t, func(context.Context, *schema.AtmosConfiguration, string) ([]string, error) {
		calls.Add(1)
		return nil, notLoggedIn
	})
	background := context.Background()

	for _, src := range []string{"oci://ghcr.io/acme/stacks:v1//catalog/vpc.yaml", "s3::https://s3.amazonaws.com/bucket/module.zip"} {
		ctx, err := WithIdentityForSource(background, &schema.AtmosConfiguration{}, src, "ops", "")
		require.NoError(t, err, src)
		assert.Equal(t, background, ctx, "the context is returned unchanged for %s", src)
		assert.Empty(t, IdentityFromContext(ctx))
	}
	assert.Zero(t, calls.Load(), "the identity is not resolved for a transport that cannot use it")

	_, err := WithIdentityForSource(background, &schema.AtmosConfiguration{}, "github.com/acme/modules//vpc", "ops", "")
	require.ErrorIs(t, err, notLoggedIn, "a Git source that names the identity still requires it")
	assert.Equal(t, int32(1), calls.Load())
}

func TestWithIdentity_RecordsTheIdentityOnTheContext(t *testing.T) {
	assert.Empty(t, IdentityFromContext(context.Background()))
	assert.False(t, FetchUsesIdentity(context.Background()))
	assert.True(t, FetchUsesIdentity(WithEnvironment(context.Background(), []string{"A=1"})))

	// An identity whose environment is empty is still the identity the fetch runs as.
	ctx := identityContext(t, nil)

	assert.Equal(t, testIdentity, IdentityFromContext(ctx))
	assert.True(t, FetchUsesIdentity(ctx))
	assert.Empty(t, EnvironmentFromContext(ctx))
}

// A broker's Git rewrite, exported into the process by an earlier ambient fetch, must not reach a
// later fetch that names its own identity: Git would rewrite the URL to carry the broker's token.
func TestApplyFetchEnvironment_NamedIdentityDoesNotInheritBrokerCredentials(t *testing.T) {
	t.Setenv("ATMOS_TEST_USER_SETTING", "kept")
	for _, name := range []string{"GIT_CONFIG_COUNT", "GIT_CONFIG_KEY_0", "GIT_CONFIG_VALUE_0"} {
		t.Setenv(name, "")
		require.NoError(t, os.Unsetenv(name))
	}
	brokerExport(t, map[string]string{
		"GIT_CONFIG_COUNT":   "1",
		"GIT_CONFIG_KEY_0":   "url.https://x-access-token:broker@github.com/acme/.insteadOf",
		"GIT_CONFIG_VALUE_0": "https://github.com/acme/",
	})

	t.Run("a named identity with no rewrite of its own runs without the broker's", func(t *testing.T) {
		cmd := exec.Command("git", "version")
		applyFetchEnvironment(identityContext(t, []string{"SSH_AUTH_SOCK=/tmp/ops.sock"}), cmd)

		for _, entry := range cmd.Env {
			assert.NotContains(t, entry, "broker@", "the broker's token must not be inherited")
			for _, exported := range []string{"GIT_CONFIG_COUNT=", "GIT_CONFIG_KEY_0=", "GIT_CONFIG_VALUE_0="} {
				assert.NotContains(t, entry, exported, "broker-exported Git configuration must not be inherited")
			}
		}
		assert.Contains(t, cmd.Env, "SSH_AUTH_SOCK=/tmp/ops.sock")
		assert.Contains(t, cmd.Env, "ATMOS_TEST_USER_SETTING=kept", "the rest of the process environment is inherited")
	})

	t.Run("a named identity's own rewrite replaces the broker's", func(t *testing.T) {
		cmd := exec.Command("git", "version")
		applyFetchEnvironment(identityContext(t, []string{
			"GIT_CONFIG_COUNT=1",
			"GIT_CONFIG_KEY_0=url.https://x-access-token:ops@github.com/acme/.insteadOf",
			"GIT_CONFIG_VALUE_0=https://github.com/acme/",
		}), cmd)

		assert.Contains(t, cmd.Env, "GIT_CONFIG_KEY_0=url.https://x-access-token:ops@github.com/acme/.insteadOf")
		assert.NotContains(t, cmd.Env, "GIT_CONFIG_KEY_0=url.https://x-access-token:broker@github.com/acme/.insteadOf")
	})

	t.Run("a fetch that names no identity keeps inheriting the broker's credentials", func(t *testing.T) {
		cmd := exec.Command("git", "version")
		applyFetchEnvironment(context.Background(), cmd)

		assert.Nil(t, cmd.Env, "the subprocess inherits the process environment, broker rewrite included")
	})
}

// The detector must see what the fetch's Git subprocess will see: for a named identity, not the
// broker's rewrite.
func TestFetchEnvironmentValue_NamedIdentityIgnoresBrokerExports(t *testing.T) {
	t.Setenv("ATMOS_TEST_FETCH_VALUE", "before")
	brokerExport(t, map[string]string{"ATMOS_TEST_FETCH_VALUE": "brokered"})
	named := identityContext(t, nil)

	assert.Equal(t, "brokered", fetchEnvironmentValue(context.Background(), "ATMOS_TEST_FETCH_VALUE"),
		"an ambient fetch reads the broker's value")
	assert.Equal(t, "before", fetchEnvironmentValue(named, "ATMOS_TEST_FETCH_VALUE"),
		"a named identity reads the value from before the broker ran")
	assert.Equal(t, "own", fetchEnvironmentValue(identityContext(t, []string{"ATMOS_TEST_FETCH_VALUE=own"}), "ATMOS_TEST_FETCH_VALUE"),
		"the identity's own value wins")
}

// URL userinfo beats every other Git credential, so an injected ambient token would silently
// replace the identity the source named.
func TestInjectToken_NamedIdentityNeverReceivesAnAmbientToken(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{Settings: schema.AtmosSettings{InjectGithubToken: true, GithubToken: "ambient-token"}}
	parse := func() *url.URL {
		parsed, err := url.Parse("https://github.com/acme/modules.git")
		require.NoError(t, err)
		return parsed
	}

	t.Run("ambient fetch gets the configured token", func(t *testing.T) {
		parsed := parse()
		NewCustomGitDetectorContext(context.Background(), atmosConfig, "").injectToken(parsed, "github.com", "github.com")

		require.NotNil(t, parsed.User, "prerequisite: injection is active for this host")
		password, _ := parsed.User.Password()
		assert.Equal(t, "ambient-token", password)
	})

	t.Run("named identity keeps the URL free of credentials", func(t *testing.T) {
		// The identity authenticates through an askpass helper: no insteadOf rewrite to detect.
		ctx := identityContext(t, []string{"GIT_ASKPASS=/usr/local/bin/ops-askpass"})
		parsed := parse()
		NewCustomGitDetectorContext(ctx, atmosConfig, "").injectToken(parsed, "github.com", "github.com")

		assert.Nil(t, parsed.User, "the ambient token must not be injected for a named identity")
	})
}

// countingBroker records how many times it was asked to provision.
type countingBroker struct{ calls *atomic.Int32 }

func (countingBroker) Name() string                            { return "counting" }
func (countingBroker) Enabled(*schema.AtmosConfiguration) bool { return true }
func (b countingBroker) Provision(context.Context, *schema.AtmosConfiguration) (map[string]string, error) {
	b.calls.Add(1)
	return nil, nil
}

// The per-fetch environment only reaches Git subprocesses. An HTTP source reads its token from the
// process environment, which is where the broker exports to, so it needs the broker even when an
// identity is named. A Git source that names an identity does not.
func TestNewClient_BrokerIsSkippedOnlyForGitSourcesThatNameAnIdentity(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		named   bool
		wantRun bool
	}{
		{name: "git source with an identity", src: "git::https://github.com/acme/modules.git", named: true, wantRun: false},
		{name: "http source with an identity", src: "https://example.com/stacks/catalog.yaml", named: true, wantRun: true},
		{name: "git source without an identity", src: "git::https://github.com/acme/modules.git", named: false, wantRun: true},
		{name: "http source without an identity", src: "https://example.com/stacks/catalog.yaml", named: false, wantRun: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(broker.SwapRegistryForTest())
			var calls atomic.Int32
			broker.Register(countingBroker{calls: &calls})
			ctx := context.Background()
			if tt.named {
				ctx = identityContext(t, []string{"SSH_AUTH_SOCK=/tmp/ops.sock"})
			}
			factory := &goGetterClientFactory{atmosConfig: &schema.AtmosConfiguration{}}

			_, err := factory.NewClient(ctx, tt.src, t.TempDir(), ClientModeAny)

			require.NoError(t, err)
			assert.Equal(t, tt.wantRun, calls.Load() == 1)
		})
	}
}
