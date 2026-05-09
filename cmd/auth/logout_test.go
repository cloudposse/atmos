package auth

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestBuildKeychainDeletionMessage covers the pure formatter for the keychain
// confirmation prompt.
func TestBuildKeychainDeletionMessage(t *testing.T) {
	got := buildKeychainDeletionMessage("prod-admin")
	// The identity name must appear in the prompt.
	assert.Contains(t, got, "prod-admin")
	// Key bullets the user is asked to confirm must be present.
	assert.Contains(t, got, "Access keys")
	assert.Contains(t, got, "Service account")
	assert.Contains(t, got, "Provider credentials")
	assert.Contains(t, got, "Session data")
}

// TestConfirmKeychainDeletion_ForceShortCircuit asserts that --force bypasses
// the prompt entirely and confirms.
func TestConfirmKeychainDeletion_ForceShortCircuit(t *testing.T) {
	confirmed, err := confirmKeychainDeletion("prod-admin", true /*force*/, false /*isTTY*/)
	require.NoError(t, err)
	assert.True(t, confirmed, "--force must short-circuit confirmation regardless of TTY state")
}

// TestConfirmKeychainDeletion_NonTTYWithoutForceErrors asserts that running
// without a TTY and without --force surfaces ErrKeychainDeletionRequiresConfirmation.
func TestConfirmKeychainDeletion_NonTTYWithoutForceErrors(t *testing.T) {
	confirmed, err := confirmKeychainDeletion("prod-admin", false /*force*/, false /*isTTY*/)
	require.Error(t, err)
	assert.False(t, confirmed)
	assert.ErrorIs(t, err, errUtils.ErrKeychainDeletionRequiresConfirmation)
}

// TestDetectExternalCredentials covers the read-only detection of provider env
// vars that may continue to authorise the caller after `atmos auth logout`.
func TestDetectExternalCredentials(t *testing.T) {
	t.Run("no env vars set returns empty", func(t *testing.T) {
		t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
		t.Setenv("AZURE_CERTIFICATE_PATH", "")
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "")

		got := detectExternalCredentials()
		assert.Empty(t, got)
	})

	t.Run("all three providers detected", func(t *testing.T) {
		t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/tmp/gcp.json")
		t.Setenv("AZURE_CERTIFICATE_PATH", "/tmp/azure.pem")
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/tmp/aws-creds")

		got := detectExternalCredentials()
		require.Len(t, got, 3)
		// Each warning should include both the env var name and the value to
		// help the user track down where the creds are coming from.
		assert.Contains(t, got[0], "GOOGLE_APPLICATION_CREDENTIALS")
		assert.Contains(t, got[0], "/tmp/gcp.json")
		assert.Contains(t, got[1], "AZURE_CERTIFICATE_PATH")
		assert.Contains(t, got[1], "/tmp/azure.pem")
		assert.Contains(t, got[2], "AWS_SHARED_CREDENTIALS_FILE")
		assert.Contains(t, got[2], "/tmp/aws-creds")
	})

	t.Run("only AWS env var set", func(t *testing.T) {
		t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
		t.Setenv("AZURE_CERTIFICATE_PATH", "")
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/aws/creds")

		got := detectExternalCredentials()
		require.Len(t, got, 1)
		assert.Contains(t, got[0], "AWS_SHARED_CREDENTIALS_FILE")
	})
}

// TestBuildLogoutOptions covers the pure construction of the interactive
// logout selection list. The output must include every identity, every
// provider, and a final "all" option in that order.
func TestBuildLogoutOptions(t *testing.T) {
	t.Run("empty config still includes the All option", func(t *testing.T) {
		got := buildLogoutOptions(nil, nil)
		require.Len(t, got, 1)
		assert.Equal(t, "all", got[0].typ)
	})

	t.Run("includes one option per identity, one per provider, plus All", func(t *testing.T) {
		identities := map[string]schema.Identity{
			"id-a": {Kind: "aws/permission-set"},
			"id-b": {Kind: "aws/assume-role"},
		}
		providers := map[string]schema.Provider{
			"sso-east": {Kind: "aws/iam-identity-center"},
		}

		got := buildLogoutOptions(identities, providers)
		// 2 identities + 1 provider + 1 "all" = 4 options.
		require.Len(t, got, 4)

		// Last option must always be "all".
		assert.Equal(t, "all", got[len(got)-1].typ,
			"the All option must be last so it appears at the bottom of the picker")

		// Collect the typ → target pairs (order of map iteration is undefined).
		seen := map[string]string{}
		for _, opt := range got {
			seen[opt.typ+":"+opt.target] = opt.label
		}
		assert.Contains(t, seen, "identity:id-a")
		assert.Contains(t, seen, "identity:id-b")
		assert.Contains(t, seen, "provider:sso-east")
		assert.Contains(t, seen, "all:")
	})

	t.Run("provider option label calls out cascade", func(t *testing.T) {
		got := buildLogoutOptions(nil, map[string]schema.Provider{
			"sso": {Kind: "aws/iam-identity-center"},
		})
		require.Len(t, got, 2)
		// First entry is the provider; "all" is last.
		assert.Equal(t, "provider", got[0].typ)
		assert.Contains(t, got[0].label, "removes all identities",
			"provider logout label must warn the user that all identities are affected")
	})
}

// TestExecuteLogoutOption_InvalidType returns ErrInvalidLogoutOption for an
// option type the dispatcher doesn't recognise.
func TestExecuteLogoutOption_InvalidType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := authTypes.NewMockAuthManager(ctrl)

	err := executeLogoutOption(context.Background(), m, logoutOption{typ: "bogus"}, false, false, false)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidLogoutOption)
}

// TestDiscoverRealms covers the realm-discovery helper used by the multi-realm
// logout flow.
func TestDiscoverRealms(t *testing.T) {
	t.Run("missing base dir returns nil without error", func(t *testing.T) {
		// Deliberately reference a path that does not exist.
		got, err := discoverRealms(filepath.Join(t.TempDir(), "does-not-exist"))
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("dirs without provider subdirs are not realms", func(t *testing.T) {
		base := t.TempDir()
		// Create a top-level dir that has NO aws/azure subdir — must not be
		// reported as a realm.
		require.NoError(t, os.MkdirAll(filepath.Join(base, "not-a-realm", "random"), 0o755))

		got, err := discoverRealms(base)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("dirs with aws subdir are reported as realms", func(t *testing.T) {
		base := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(base, "realm-1", "aws"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(base, "realm-2", "azure"), 0o755))
		// File at top-level must be ignored.
		require.NoError(t, os.WriteFile(filepath.Join(base, "stray-file"), []byte("x"), 0o644))

		got, err := discoverRealms(base)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"realm-1", "realm-2"}, got,
			"realms are directories containing an aws/ or azure/ provider subdir")
	})
}
