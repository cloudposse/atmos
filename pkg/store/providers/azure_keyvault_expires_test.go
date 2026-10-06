package providers

import (
	"context"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storepkg "github.com/cloudposse/atmos/pkg/store"
)

// Compile-time sentinels so a rename of the option or SDK fields these tests rely on fails the build.
var (
	_ = AzureKeyVaultStoreOptions{Tags: map[string]string{}, Expires: new(string)}
	_ = azsecrets.SetSecretParameters{Tags: map[string]*string{}, SecretAttributes: &azsecrets.SecretAttributes{}}
)

// azureTestStoreWithOptions builds a store from options with a capturing mock client. Identity
// deferral keeps construction free of any credential chain lookups.
func azureTestStoreWithOptions(t *testing.T, opts AzureKeyVaultStoreOptions) (*AzureKeyVaultStore, *[]azsecrets.SetSecretParameters) {
	t.Helper()

	opts.VaultURL = "https://test.vault.azure.net"
	s, err := NewAzureKeyVaultStore(opts, "test-identity")
	require.NoError(t, err)

	akv, ok := s.(*AzureKeyVaultStore)
	require.True(t, ok)

	var captured []azsecrets.SetSecretParameters
	akv.client = &mockClient{setSecretFunc: func(_ context.Context, _ string, parameters azsecrets.SetSecretParameters, _ *azsecrets.SetSecretOptions) (azsecrets.SetSecretResponse, error) {
		captured = append(captured, parameters)
		return azsecrets.SetSecretResponse{}, nil
	}}

	return akv, &captured
}

func TestParseAzureExpires(t *testing.T) {
	rfc := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		input    string
		wantTime *time.Time
		wantDur  time.Duration
		wantErr  error
	}{
		{name: "rfc3339 timestamp", input: "2027-01-01T00:00:00Z", wantTime: &rfc},
		{name: "rfc3339 with offset normalized to utc", input: "2027-01-01T02:00:00+02:00", wantTime: &rfc},
		{name: "date only is midnight utc", input: "2027-01-01", wantTime: &rfc},
		{name: "day suffix", input: "90d", wantDur: 90 * 24 * time.Hour},
		{name: "single day", input: "1d", wantDur: 24 * time.Hour},
		{name: "go hours", input: "2160h", wantDur: 2160 * time.Hour},
		{name: "go compound", input: "720h30m", wantDur: 720*time.Hour + 30*time.Minute},
		{name: "surrounding whitespace trimmed", input: "  90d ", wantDur: 90 * 24 * time.Hour},
		{name: "empty", input: "", wantErr: storepkg.ErrInvalidExpires},
		{name: "garbage", input: "soon", wantErr: storepkg.ErrInvalidExpires},
		{name: "zero days", input: "0d", wantErr: storepkg.ErrInvalidExpires},
		{name: "zero hours", input: "0h", wantErr: storepkg.ErrInvalidExpires},
		{name: "bare zero", input: "0", wantErr: storepkg.ErrInvalidExpires},
		{name: "negative duration", input: "-5h", wantErr: storepkg.ErrInvalidExpires},
		{name: "sub-second duration", input: "500ms", wantErr: storepkg.ErrInvalidExpires},
		{name: "just under one second", input: "999ms", wantErr: storepkg.ErrInvalidExpires},
		{name: "one second boundary", input: "1s", wantDur: time.Second},
		{name: "negative days", input: "-5d", wantErr: storepkg.ErrInvalidExpires},
		{name: "fractional days", input: "1.5d", wantErr: storepkg.ErrInvalidExpires},
		{name: "day overflow", input: "99999999999999d", wantErr: storepkg.ErrInvalidExpires},
		{name: "invalid date", input: "2027-13-45", wantErr: storepkg.ErrInvalidExpires},
		{name: "day suffix without number", input: "d", wantErr: storepkg.ErrInvalidExpires},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTime, gotDur, err := parseAzureExpires(tt.input)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, gotTime)
				assert.Zero(t, gotDur)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantDur, gotDur)
			if tt.wantTime == nil {
				assert.Nil(t, gotTime)
				return
			}
			require.NotNil(t, gotTime)
			assert.True(t, tt.wantTime.Equal(*gotTime), "want %s, got %s", tt.wantTime, gotTime)
		})
	}
}

func TestNewAzureKeyVaultStore_InvalidExpires(t *testing.T) {
	for _, input := range []string{"", "soon", "0d", "0h", "-5h", "500ms"} {
		t.Run("expires="+input, func(t *testing.T) {
			s, err := NewAzureKeyVaultStore(AzureKeyVaultStoreOptions{
				VaultURL: "https://test.vault.azure.net",
				Expires:  ptrTo(input),
			}, "test-identity")
			require.ErrorIs(t, err, storepkg.ErrInvalidExpires)
			assert.Nil(t, s)
		})
	}
}

func TestNewAzureKeyVaultStore_PastExpiresAccepted(t *testing.T) {
	s, err := NewAzureKeyVaultStore(AzureKeyVaultStoreOptions{
		VaultURL: "https://test.vault.azure.net",
		Expires:  ptrTo("2000-01-01"),
	}, "test-identity")
	require.NoError(t, err)

	akv, ok := s.(*AzureKeyVaultStore)
	require.True(t, ok)
	require.NotNil(t, akv.expiresAt)
	assert.Equal(t, 2000, akv.expiresAt.Year())
}

func TestAzureKeyVaultStore_Set_TagsAndFixedExpires(t *testing.T) {
	want := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		expires string
	}{
		{name: "rfc3339", expires: "2027-01-01T00:00:00Z"},
		{name: "date only", expires: "2027-01-01"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, captured := azureTestStoreWithOptions(t, AzureKeyVaultStoreOptions{
				Tags:    map[string]string{"managed-by": "atmos", "environment": "prod"},
				Expires: ptrTo(tt.expires),
			})

			require.NoError(t, s.Set("dev", "app", "secret", "value"))
			require.Len(t, *captured, 1)

			params := (*captured)[0]
			require.NotNil(t, params.Value)
			assert.Equal(t, `"value"`, *params.Value)

			require.Len(t, params.Tags, 2)
			require.NotNil(t, params.Tags["managed-by"])
			assert.Equal(t, "atmos", *params.Tags["managed-by"])
			require.NotNil(t, params.Tags["environment"])
			assert.Equal(t, "prod", *params.Tags["environment"])

			require.NotNil(t, params.SecretAttributes)
			require.NotNil(t, params.SecretAttributes.Expires)
			assert.True(t, want.Equal(*params.SecretAttributes.Expires), "want %s, got %s", want, params.SecretAttributes.Expires)
		})
	}
}

func TestAzureKeyVaultStore_Set_RelativeExpiresRecomputed(t *testing.T) {
	tests := []struct {
		name    string
		expires string
		want    time.Duration
	}{
		{name: "days", expires: "90d", want: 90 * 24 * time.Hour},
		{name: "go hours", expires: "2160h", want: 2160 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, captured := azureTestStoreWithOptions(t, AzureKeyVaultStoreOptions{Expires: ptrTo(tt.expires)})

			before := time.Now().UTC()
			require.NoError(t, s.Set("dev", "app", "secret", "one"))
			// Sleep so the second write's clock reading is strictly later on coarse timers (Windows).
			time.Sleep(20 * time.Millisecond)
			require.NoError(t, s.Set("dev", "app", "secret", "two"))
			after := time.Now().UTC()

			require.Len(t, *captured, 2)
			first := (*captured)[0].SecretAttributes
			second := (*captured)[1].SecretAttributes
			require.NotNil(t, first)
			require.NotNil(t, first.Expires)
			require.NotNil(t, second)
			require.NotNil(t, second.Expires)

			assert.False(t, first.Expires.Before(before.Add(tt.want)), "first expiry earlier than now+duration")
			assert.False(t, second.Expires.After(after.Add(tt.want)), "second expiry later than now+duration")
			assert.True(t, second.Expires.After(*first.Expires), "relative expiry must be recomputed on every write")
			assert.Nil(t, (*captured)[0].Tags)
		})
	}
}

func TestAzureKeyVaultStore_Set_NoTagsNoExpires(t *testing.T) {
	tests := []struct {
		name string
		opts AzureKeyVaultStoreOptions
	}{
		{name: "unset", opts: AzureKeyVaultStoreOptions{}},
		{name: "empty tags map", opts: AzureKeyVaultStoreOptions{Tags: map[string]string{}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, captured := azureTestStoreWithOptions(t, tt.opts)

			require.NoError(t, s.Set("dev", "app", "secret", "value"))
			require.Len(t, *captured, 1)
			assert.Nil(t, (*captured)[0].Tags)
			assert.Nil(t, (*captured)[0].SecretAttributes)
		})
	}
}

func TestAzureKeyVaultStore_Set_TagsOnlyAndExpiresOnly(t *testing.T) {
	t.Run("tags only", func(t *testing.T) {
		s, captured := azureTestStoreWithOptions(t, AzureKeyVaultStoreOptions{Tags: map[string]string{"a": "b"}})
		require.NoError(t, s.Set("", "", "secret", "value"))
		require.Len(t, *captured, 1)
		require.Len(t, (*captured)[0].Tags, 1)
		assert.Nil(t, (*captured)[0].SecretAttributes)
	})

	t.Run("expires only", func(t *testing.T) {
		s, captured := azureTestStoreWithOptions(t, AzureKeyVaultStoreOptions{Expires: ptrTo("2027-01-01")})
		require.NoError(t, s.Set("", "", "secret", "value"))
		require.Len(t, *captured, 1)
		assert.Nil(t, (*captured)[0].Tags)
		require.NotNil(t, (*captured)[0].SecretAttributes)
		require.NotNil(t, (*captured)[0].SecretAttributes.Expires)
	})
}

func TestAzureKeyVaultStore_TagsMutationIsolation(t *testing.T) {
	t.Run("options map mutated after construction", func(t *testing.T) {
		tags := map[string]string{"managed-by": "atmos"}
		s, captured := azureTestStoreWithOptions(t, AzureKeyVaultStoreOptions{Tags: tags})

		tags["managed-by"] = "someone-else"
		tags["injected"] = "yes"

		require.NoError(t, s.Set("dev", "app", "secret", "value"))
		require.Len(t, *captured, 1)
		require.Len(t, (*captured)[0].Tags, 1)
		assert.Equal(t, "atmos", *(*captured)[0].Tags["managed-by"])
	})

	t.Run("params mutated by the sdk after write", func(t *testing.T) {
		s, captured := azureTestStoreWithOptions(t, AzureKeyVaultStoreOptions{Tags: map[string]string{"managed-by": "atmos"}})

		require.NoError(t, s.Set("dev", "app", "secret", "one"))
		require.Len(t, *captured, 1)
		*(*captured)[0].Tags["managed-by"] = "mutated"
		(*captured)[0].Tags["extra"] = new(string)

		require.NoError(t, s.Set("dev", "app", "secret", "two"))
		require.Len(t, *captured, 2)
		require.Len(t, (*captured)[1].Tags, 1)
		assert.Equal(t, "atmos", *(*captured)[1].Tags["managed-by"])
	})
}

func TestBuildAzureKeyVaultStore_TagsAndExpiresFromConfig(t *testing.T) {
	s, err := buildAzureKeyVaultStore("prod/azure", storepkg.StoreConfig{
		Kind:     storepkg.KindAzureKeyVault,
		Identity: "test-identity",
		Options: map[string]interface{}{
			"vault_url": "https://test.vault.azure.net",
			"tags": map[string]interface{}{
				"managed-by":  "atmos",
				"environment": "prod",
			},
			"expires": "90d",
		},
	})
	require.NoError(t, err)

	akv, ok := s.(*AzureKeyVaultStore)
	require.True(t, ok)
	require.Len(t, akv.tags, 2)
	assert.Equal(t, "atmos", *akv.tags["managed-by"])
	assert.Equal(t, "prod", *akv.tags["environment"])
	assert.Equal(t, 90*24*time.Hour, akv.expiresIn)
	assert.Nil(t, akv.expiresAt)
}

func TestBuildAzureKeyVaultStore_InvalidExpiresFromConfig(t *testing.T) {
	_, err := buildAzureKeyVaultStore("prod/azure", storepkg.StoreConfig{
		Kind:     storepkg.KindAzureKeyVault,
		Identity: "test-identity",
		Options: map[string]interface{}{
			"vault_url": "https://test.vault.azure.net",
			"expires":   "never",
		},
	})
	require.ErrorIs(t, err, storepkg.ErrInvalidExpires)
}

func TestBuildAzureKeyVaultStore_NonStringTagValueRejected(t *testing.T) {
	_, err := buildAzureKeyVaultStore("prod/azure", storepkg.StoreConfig{
		Kind:     storepkg.KindAzureKeyVault,
		Identity: "test-identity",
		Options: map[string]interface{}{
			"vault_url": "https://test.vault.azure.net",
			"tags":      map[string]interface{}{"count": []string{"x"}},
		},
	})
	require.ErrorIs(t, err, storepkg.ErrParseAzureKeyVaultOptions)
}
