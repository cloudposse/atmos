package main

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGitRefFromVersion(t *testing.T) {
	cases := []struct {
		name    string
		version string
		want    string
	}{
		{"tagged release", "v1.0.2", "v1.0.2"},
		{"pseudo-version resolves to commit hash", "v0.0.0-20240101000000-abcdef012345", "abcdef012345"},
		{"submodule tagged release keeps prefix", "auth/v0.20.0", "auth/v0.20.0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, gitRefFromVersion(tc.version))
		})
	}
}

func TestApplyOverridesRewritesMatchingEntries(t *testing.T) {
	entries := []LicenseEntry{
		{Module: "dario.cat/mergo", URL: "https://go-licenses-unresolved.example", License: "BSD-3-Clause"},
		{Module: "github.com/aws/aws-sdk-go-v2/internal/endpoints/v2", URL: "unknown", License: "Apache-2.0"},
		{Module: "github.com/not/overridden", URL: "https://original.example/LICENSE", License: "MIT"},
	}

	versions := map[string]string{
		"dario.cat/mergo": "v1.0.2",
		"github.com/aws/aws-sdk-go-v2/internal/endpoints/v2": "v1.3.4",
	}

	applyOverrides(entries, func(module string) (string, error) {
		v, ok := versions[module]
		if !ok {
			return "", errors.New("unexpected module: " + module)
		}
		return v, nil
	})

	assert.Equal(t, "https://github.com/imdario/mergo/blob/v1.0.2/LICENSE", entries[0].URL)
	assert.Equal(t, "https://github.com/aws/aws-sdk-go-v2/blob/internal/endpoints/v1.3.4/internal/endpoints/v2/LICENSE.txt", entries[1].URL)
	assert.Equal(t, "https://original.example/LICENSE", entries[2].URL, "non-overridden entries must be left untouched")
}

func TestApplyOverridesUsesPseudoVersionCommitHash(t *testing.T) {
	entries := []LicenseEntry{
		{Module: "go4.org/intern", URL: "unknown", License: "BSD-3-Clause"},
	}

	applyOverrides(entries, func(string) (string, error) {
		return "v0.0.0-20220617035311-6f0d2f27862a", nil
	})

	assert.Equal(t, "https://github.com/go4org/intern/blob/6f0d2f27862a/LICENSE", entries[0].URL)
}

func TestApplyOverridesSkipsUnresolvableVersion(t *testing.T) {
	original := "https://go-licenses-unresolved.example"
	entries := []LicenseEntry{
		{Module: "dario.cat/mergo", URL: original, License: "BSD-3-Clause"},
	}

	applyOverrides(entries, func(string) (string, error) {
		return "", errors.New("go list failed")
	})

	assert.Equal(t, original, entries[0].URL, "a failed version lookup must leave the entry's URL untouched")
}

func TestApplyOverridesSkipsModuleNotInEntries(t *testing.T) {
	entries := []LicenseEntry{
		{Module: "github.com/not/dario", URL: "https://original.example/LICENSE", License: "MIT"},
	}

	calls := 0
	applyOverrides(entries, func(string) (string, error) {
		calls++
		return "v1.0.0", nil
	})

	assert.Equal(t, 0, calls, "moduleVersion must not be called for modules with no override rule")
	assert.Equal(t, "https://original.example/LICENSE", entries[0].URL)
}
