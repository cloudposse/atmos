package main

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsGoogleCloudGoModule(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"cloud.google.com/go", true},
		{"cloud.google.com/go/storage", true},
		{"cloud.google.com/go/auth/oauth2adapt", true},
		{"cloud.google.com/goo", false},
		{"github.com/googleapis/gax-go/v2", false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, isGoogleCloudGoModule(tc.path), "isGoogleCloudGoModule(%q)", tc.path)
	}
}

func TestApplyGoogleCloudGoOverridesRootModule(t *testing.T) {
	entries := []LicenseEntry{
		{Module: "cloud.google.com/go", URL: "unknown", License: "Apache-2.0"},
	}

	err := applyGoogleCloudGoOverrides(entries, func() ([]ModuleVersion, error) {
		return []ModuleVersion{{Path: "cloud.google.com/go", Version: "v0.123.0"}}, nil
	})

	require.NoError(t, err)
	assert.Equal(t, "https://github.com/googleapis/google-cloud-go/blob/v0.123.0/LICENSE", entries[0].URL)
}

func TestApplyGoogleCloudGoOverridesSubmodule(t *testing.T) {
	entries := []LicenseEntry{
		{Module: "cloud.google.com/go/auth", URL: "unknown", License: "Apache-2.0"},
	}

	err := applyGoogleCloudGoOverrides(entries, func() ([]ModuleVersion, error) {
		return []ModuleVersion{{Path: "cloud.google.com/go/auth", Version: "v0.20.0"}}, nil
	})

	require.NoError(t, err)
	assert.Equal(t, "https://github.com/googleapis/google-cloud-go/blob/auth/v0.20.0/auth/LICENSE", entries[0].URL)
}

func TestApplyGoogleCloudGoOverridesPseudoVersion(t *testing.T) {
	entries := []LicenseEntry{
		{Module: "cloud.google.com/go/iam", URL: "unknown", License: "Apache-2.0"},
	}

	err := applyGoogleCloudGoOverrides(entries, func() ([]ModuleVersion, error) {
		return []ModuleVersion{{Path: "cloud.google.com/go/iam", Version: "v0.0.0-20240101000000-abcdef012345"}}, nil
	})

	require.NoError(t, err)
	assert.Equal(t, "https://github.com/googleapis/google-cloud-go/blob/iam/abcdef012345/iam/LICENSE", entries[0].URL)
}

func TestApplyGoogleCloudGoOverridesSkipsModuleNotInEntries(t *testing.T) {
	entries := []LicenseEntry{
		{Module: "github.com/other/module", URL: "https://original.example/LICENSE", License: "MIT"},
	}

	err := applyGoogleCloudGoOverrides(entries, func() ([]ModuleVersion, error) {
		return []ModuleVersion{{Path: "cloud.google.com/go/storage", Version: "v1.0.0"}}, nil
	})

	require.NoError(t, err)
	assert.Equal(t, "https://original.example/LICENSE", entries[0].URL)
}

func TestApplyGoogleCloudGoOverridesPropagatesListError(t *testing.T) {
	wantErr := errors.New("go list failed")
	err := applyGoogleCloudGoOverrides(nil, func() ([]ModuleVersion, error) {
		return nil, wantErr
	})

	require.ErrorIs(t, err, wantErr)
}
