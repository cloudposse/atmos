package tfvars

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// secretSet returns an isSecret predicate that reports true when the input contains
// any of the given secret literals as a substring (mirroring io.ContainsSecret).
func secretSet(secrets ...string) func(string) bool {
	return func(s string) bool {
		for _, sec := range secrets {
			if sec != "" && strings.Contains(s, sec) {
				return true
			}
		}
		return false
	}
}

func TestPartition(t *testing.T) {
	const secret = "s3cr3t-value"

	tests := []struct {
		name       string
		vars       map[string]any
		isSecret   func(string) bool
		wantSafe   []string // top-level keys expected in safe
		wantSecret []string // top-level keys expected in secret
	}{
		{
			name:       "direct secret scalar",
			vars:       map[string]any{"password": secret, "region": "us-east-1"},
			isSecret:   secretSet(secret),
			wantSafe:   []string{"region"},
			wantSecret: []string{"password"},
		},
		{
			name:       "secret as substring of composed string",
			vars:       map[string]any{"db_url": "postgres://user:" + secret + "@host/db", "name": "vpc"},
			isSecret:   secretSet(secret),
			wantSafe:   []string{"name"},
			wantSecret: []string{"db_url"},
		},
		{
			name:       "secret nested in map",
			vars:       map[string]any{"config": map[string]any{"token": secret, "ttl": 30}, "flag": true},
			isSecret:   secretSet(secret),
			wantSafe:   []string{"flag"},
			wantSecret: []string{"config"},
		},
		{
			name:       "secret nested in list",
			vars:       map[string]any{"args": []any{"--flag", secret}, "count": 3},
			isSecret:   secretSet(secret),
			wantSafe:   []string{"count"},
			wantSecret: []string{"args"},
		},
		{
			name:       "numeric secret",
			vars:       map[string]any{"pin": 1234, "port": 8080},
			isSecret:   secretSet("1234"),
			wantSafe:   []string{"port"},
			wantSecret: []string{"pin"},
		},
		{
			name:       "no secrets",
			vars:       map[string]any{"a": "1", "b": 2},
			isSecret:   secretSet(secret),
			wantSafe:   []string{"a", "b"},
			wantSecret: []string{},
		},
		{
			name:       "nil predicate keeps everything safe",
			vars:       map[string]any{"password": secret},
			isSecret:   nil,
			wantSafe:   []string{"password"},
			wantSecret: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			safe, secretMap := Partition(tt.vars, tt.isSecret)

			require.NotNil(t, safe)
			require.NotNil(t, secretMap)
			assert.ElementsMatch(t, tt.wantSafe, keys(safe), "safe keys")
			assert.ElementsMatch(t, tt.wantSecret, keys(secretMap), "secret keys")

			// Values must be preserved unchanged for the keys that land in each map.
			for _, k := range tt.wantSafe {
				assert.Equal(t, tt.vars[k], safe[k])
			}
			for _, k := range tt.wantSecret {
				assert.Equal(t, tt.vars[k], secretMap[k])
			}
		})
	}
}

func TestPartition_DoesNotMutateInput(t *testing.T) {
	const secret = "do-not-mutate"
	in := map[string]any{"password": secret, "region": "us-east-1"}
	Partition(in, secretSet(secret))

	assert.Equal(t, map[string]any{"password": secret, "region": "us-east-1"}, in)
}

func TestSecretEnv(t *testing.T) {
	env, err := SecretEnv(map[string]any{
		"password": "p@ss",
		"port":     8080,
		"enabled":  true,
		"tags":     map[string]any{"env": "prod"},
		"hosts":    []any{"a", "b"},
	})
	require.NoError(t, err)

	got := make(map[string]string, len(env))
	for _, e := range env {
		k, v, ok := strings.Cut(e, "=")
		require.True(t, ok, "entry %q must contain =", e)
		got[k] = v
	}

	// String passes through verbatim.
	assert.Equal(t, "p@ss", got["TF_VAR_password"])
	// Scalars JSON-encoded (valid HCL).
	assert.Equal(t, "8080", got["TF_VAR_port"])
	assert.Equal(t, "true", got["TF_VAR_enabled"])
	// Complex types JSON-encoded.
	assert.Equal(t, `{"env":"prod"}`, got["TF_VAR_tags"])
	assert.Equal(t, `["a","b"]`, got["TF_VAR_hosts"])
}

func TestSecretEnv_Empty(t *testing.T) {
	env, err := SecretEnv(map[string]any{})
	require.NoError(t, err)
	assert.Empty(t, env)
}

func TestSecretEnv_AllPrefixed(t *testing.T) {
	env, err := SecretEnv(map[string]any{"a": "1", "b": "2"})
	require.NoError(t, err)
	sort.Strings(env)
	require.Len(t, env, 2)
	for _, e := range env {
		assert.True(t, strings.HasPrefix(e, "TF_VAR_"), "entry %q must be TF_VAR_ prefixed", e)
	}
	assert.Equal(t, "TF_VAR_a=1", env[0])
	assert.Equal(t, "TF_VAR_b=2", env[1])
}

func keys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
