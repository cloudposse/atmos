package env

import (
	"os"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

// fakeEnv is an in-memory environment used to test the promotion policy without
// touching the real process environment.
type fakeEnv struct {
	vars map[string]string
}

func newFakeEnv(initial map[string]string) *fakeEnv {
	vars := make(map[string]string, len(initial))
	for k, v := range initial {
		vars[k] = v
	}
	return &fakeEnv{vars: vars}
}

func (f *fakeEnv) lookup(key string) (string, bool) {
	v, ok := f.vars[key]
	return v, ok
}

func (f *fakeEnv) set(key, value string) error {
	f.vars[key] = value
	return nil
}

func TestPromoteAtmosEnvWith_OnlyAtmosPrefixedKeys(t *testing.T) {
	fake := newFakeEnv(nil)

	promoted := PromoteAtmosEnvWith(map[string]string{
		"ATMOS_PROFILE":   "prod",
		"ATMOS_BASE_PATH": "/repo",
		"AWS_REGION":      "us-east-1", // not promoted: no ATMOS_ prefix.
		"PATH":            "/usr/bin",  // not promoted.
	}, fake.lookup, fake.set)

	sort.Strings(promoted)
	assert.Equal(t, []string{"ATMOS_BASE_PATH", "ATMOS_PROFILE"}, promoted)

	// ATMOS_ keys were set with their values.
	assert.Equal(t, "prod", fake.vars["ATMOS_PROFILE"])
	assert.Equal(t, "/repo", fake.vars["ATMOS_BASE_PATH"])
	// Non-ATMOS keys were never touched.
	_, ok := fake.vars["AWS_REGION"]
	assert.False(t, ok, "AWS_REGION must not be promoted")
	_, ok = fake.vars["PATH"]
	assert.False(t, ok, "PATH must not be promoted")
}

func TestPromoteAtmosEnvWith_RealEnvWins(t *testing.T) {
	// ATMOS_PROFILE already set in the (fake) real environment.
	fake := newFakeEnv(map[string]string{"ATMOS_PROFILE": "from-shell"})

	promoted := PromoteAtmosEnvWith(map[string]string{
		"ATMOS_PROFILE":   "from-dotenv", // must NOT override the existing value.
		"ATMOS_BASE_PATH": "/repo",       // not yet set -> promoted.
	}, fake.lookup, fake.set)

	assert.Equal(t, []string{"ATMOS_BASE_PATH"}, promoted)
	// Existing value preserved, not overwritten.
	assert.Equal(t, "from-shell", fake.vars["ATMOS_PROFILE"])
	assert.Equal(t, "/repo", fake.vars["ATMOS_BASE_PATH"])
}

func TestPromoteAtmosEnvWith_EmptyExistingValueStillWins(t *testing.T) {
	// A var that is set but empty must still count as "set" (real env wins),
	// matching os.LookupEnv semantics.
	fake := newFakeEnv(map[string]string{"ATMOS_PROFILE": ""})

	promoted := PromoteAtmosEnvWith(map[string]string{
		"ATMOS_PROFILE": "from-dotenv",
	}, fake.lookup, fake.set)

	assert.Nil(t, promoted)
	assert.Equal(t, "", fake.vars["ATMOS_PROFILE"], "explicitly-empty real env must not be overwritten")
}

func TestPromoteAtmosEnvWith_CasePreserved(t *testing.T) {
	fake := newFakeEnv(nil)

	promoted := PromoteAtmosEnvWith(map[string]string{
		"ATMOS_PROFILE": "prod",
		// Lowercase atmos_ does NOT match the case-sensitive ATMOS_ prefix.
		"atmos_profile": "ignored",
	}, fake.lookup, fake.set)

	assert.Equal(t, []string{"ATMOS_PROFILE"}, promoted)
	_, ok := fake.vars["atmos_profile"]
	assert.False(t, ok, "lowercase atmos_ key must not be promoted")
}

func TestPromoteAtmosEnvWith_EmptyMap(t *testing.T) {
	fake := newFakeEnv(nil)
	assert.Nil(t, PromoteAtmosEnvWith(nil, fake.lookup, fake.set))
	assert.Nil(t, PromoteAtmosEnvWith(map[string]string{}, fake.lookup, fake.set))
}

func TestPromoteAtmosEnv_SetsRealProcessEnv(t *testing.T) {
	const key = "ATMOS_TEST_PROMOTE_KEY"

	// Guarantee the key is unset going in, and restore it after the test so we
	// don't leak into other tests (os.Setenv inside PromoteAtmosEnv is not tracked
	// by t.Setenv).
	orig, had := os.LookupEnv(key)
	_ = os.Unsetenv(key)
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(key, orig)
		} else {
			_ = os.Unsetenv(key)
		}
	})

	promoted := PromoteAtmosEnv(map[string]string{key: "value", "AWS_REGION": "us-east-1"})
	assert.Equal(t, []string{key}, promoted)
	assert.Equal(t, "value", os.Getenv(key))
	// Sanity: the non-ATMOS key was not promoted into the real env.
	_, ok := os.LookupEnv("AWS_REGION_SHOULD_NOT_EXIST")
	assert.False(t, ok)
}
