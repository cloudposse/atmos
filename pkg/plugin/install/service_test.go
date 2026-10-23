package install

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/oci/signature"
	"github.com/cloudposse/atmos/pkg/oci/signature/signaturetest"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	"github.com/cloudposse/atmos/pkg/schema"
)

//go:generate go run go.uber.org/mock/mockgen -destination=mock_registry_test.go -package=install github.com/cloudposse/atmos/pkg/oci Registry

func TestVersionSelectionAndPinnedReferences(t *testing.T) {
	registry := NewMockRegistry(gomock.NewController(t))
	service := &Service{Registry: registry}
	repository, err := name.NewRepository("ghcr.io/acme/plugin")
	require.NoError(t, err)
	registry.EXPECT().Tags(gomock.Any(), repository).Return([]string{"v1.2.0", "1.2.0", "latest", "2.0.0", "1.1.0"}, nil)
	selected, err := service.selectVersion(t.Context(), repository, "^1.0.0")
	require.NoError(t, err)
	require.Equal(t, "1.2.0", selected)
	_, err = service.selectVersion(t.Context(), repository, "no version")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	registry.EXPECT().Tags(gomock.Any(), repository).Return(nil, os.ErrPermission)
	_, err = service.selectVersion(t.Context(), repository, "*")
	require.ErrorIs(t, err, os.ErrPermission)
	registry.EXPECT().Tags(gomock.Any(), repository).Return([]string{"latest", "2.0.0"}, nil)
	_, err = service.selectVersion(t.Context(), repository, "^1")
	require.ErrorIs(t, err, errUtils.ErrPluginVersionNotFound)
	digest := "sha256:" + strings.Repeat("a", 64)
	ref, err := service.resolveReference(t.Context(), repository, &Request{PinnedDigest: digest})
	require.NoError(t, err)
	require.Equal(t, repository.Digest(digest).Name(), ref.Name())
	_, err = service.resolveReference(t.Context(), repository, &Request{PinnedDigest: "invalid"})
	require.Error(t, err)
	for _, tc := range []struct{ version, constraint string }{{"1.0.0", "invalid"}, {"invalid", "*"}, {"1.0.0", ">2"}} {
		require.Error(t, versionMatches(tc.version, tc.constraint))
	}
}

func installFixture(t *testing.T) (*Service, *pluginoci.Release, string) {
	t.Helper()
	directory := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(directory, "bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "bin", "plugin"), []byte("compiled executable"), 0o755))
	manifest := `apiVersion: atmos/v1
kind: AtmosPluginConfig
metadata: {name: acme/fixture, version: 1.0.0}
spec:
  runtime: {type: native, protocol: 1, executable: bin/plugin}
  compatibility: {protocol: ">=1 <2"}
  platforms:
    PLATFORM: {executable: bin/plugin}
  capabilities:
    fixtures:
      acme/fixture: {}
`
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(strings.ReplaceAll(manifest, "PLATFORM", CurrentPlatform())), 0o644))
	descriptors := extension.New()
	require.NoError(t, descriptors.Register(extension.Descriptor{Capability: "fixture", Path: []string{"fixtures"}, Version: 1, Validate: func(extension.Binding) error { return nil }}))
	index, err := pluginoci.BuildIndex(directory, descriptors)
	require.NoError(t, err)
	release, err := pluginoci.Inspect(index, "")
	require.NoError(t, err)
	cache, err := NewCache(t.TempDir())
	require.NoError(t, err)
	service := &Service{Config: &schema.AtmosConfiguration{}, Descriptors: descriptors, Cache: cache, Platform: CurrentPlatform()}
	return service, release, directory
}

func TestCacheReusesVerifiedBytesAndRejectsTampering(t *testing.T) {
	service, release, _ := installFixture(t)
	path, err := service.Cache.Store(t.Context(), release, CurrentPlatform(), service.Descriptors)
	require.NoError(t, err)
	second, err := service.Cache.Store(t.Context(), release, CurrentPlatform(), service.Descriptors)
	require.NoError(t, err)
	require.Equal(t, path, second)
	require.NoError(t, os.WriteFile(filepath.Join(path, "bin", "plugin"), []byte("tampered executable"), 0o755))
	// Tampered bytes are quarantined and replaced by the verified release (see cache_repair_test.go).
	repaired, err := service.Cache.Store(t.Context(), release, CurrentPlatform(), service.Descriptors)
	require.NoError(t, err)
	require.NoError(t, release.VerifyInstalled(CurrentPlatform(), repaired))
	_, err = service.Cache.Store(t.Context(), release, "unknown/arch", service.Descriptors)
	require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedPlatform)
	for _, platform := range []string{"../escape", `linux\amd64`} {
		_, err = service.Cache.Store(t.Context(), release, platform, service.Descriptors)
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	}
	_, err = service.Cache.Installed(&pluginlock.Entry{IndexDigest: "invalid"}, CurrentPlatform())
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	service.Cache.Directory = filepath.Join(path, "bin", "plugin")
	_, err = service.Cache.Store(t.Context(), release, CurrentPlatform(), service.Descriptors)
	require.Error(t, err)
}

func TestPrepareLayoutVerifiesSelectedVersionAndLock(t *testing.T) {
	service, release, directory := installFixture(t)
	layout := filepath.Join(t.TempDir(), "layout")
	require.NoError(t, pluginoci.Pack(directory, layout, service.Descriptors))
	request := &Request{Source: "oci://ghcr.io/acme/plugin", Constraint: "*", Layout: layout}
	entry, err := service.Prepare(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, release.Digest, entry.IndexDigest)
	request.Locked = entry
	restored, err := service.Prepare(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, entry, restored)
	request.Source = "oci://ghcr.io/other/plugin"
	_, err = service.Prepare(t.Context(), request)
	require.ErrorIs(t, err, errUtils.ErrPluginFrozenDrift)
	request.Locked = nil
	request.SelectedVersion = "2.0.0"
	_, err = service.Prepare(t.Context(), request)
	require.ErrorIs(t, err, errUtils.ErrOCIIntegrity)
	request.SelectedVersion = ""
	request.Constraint = ">2"
	_, err = service.Prepare(t.Context(), request)
	require.ErrorIs(t, err, errUtils.ErrPluginVersionNotFound)
	request.Constraint = "*"
	request.Layout = filepath.Join(t.TempDir(), "absent")
	_, err = service.Prepare(t.Context(), request)
	require.Error(t, err)
	request.Layout = ""
	request.Source = "not valid"
	_, err = service.Prepare(t.Context(), request)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	request.Source = "oci://ghcr.io/acme/plugin"
	registry := NewMockRegistry(gomock.NewController(t))
	service.Registry = registry
	registry.EXPECT().Tags(gomock.Any(), gomock.Any()).Return(nil, os.ErrPermission)
	_, err = service.Prepare(t.Context(), request)
	require.ErrorIs(t, err, os.ErrPermission)
	request.PinnedDigest = release.Digest
	registry.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, os.ErrPermission)
	_, err = service.Prepare(t.Context(), request)
	require.ErrorIs(t, err, os.ErrPermission)
}

func TestNewServiceValidatesConfiguration(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}
	cfg.Plugins.CacheDir = t.TempDir()
	service, err := NewService(cfg, extension.New())
	require.NoError(t, err)
	require.Equal(t, CurrentPlatform(), service.Platform)
	cfg.Plugins.Policy.CacheTTL = "1h"
	_, err = NewService(cfg, extension.New())
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
}

func TestVersionSelectionHonorsExactPinsAndPrereleaseOptIn(t *testing.T) {
	repository, err := name.NewRepository("ghcr.io/acme/plugin")
	require.NoError(t, err)
	for _, tc := range []struct{ constraint, want string }{{"1.0.0", "1.0.0"}, {"^1.0.0", "1.1.0"}, {">=1.0.0-0 <2.0.0", "1.2.0-rc.1"}} {
		t.Run(tc.constraint, func(t *testing.T) {
			registry := NewMockRegistry(gomock.NewController(t))
			registry.EXPECT().Tags(gomock.Any(), repository).Return([]string{"1.0.0", "1.1.0", "1.2.0-rc.1", "2.0.0"}, nil)
			service := &Service{Registry: registry}
			selected, err := service.selectVersion(t.Context(), repository, tc.constraint)
			require.NoError(t, err)
			require.Equal(t, tc.want, selected)
		})
	}
}

func TestProjectsShareXDGCacheWithoutRegistryAccess(t *testing.T) {
	t.Setenv("ATMOS_XDG_DATA_HOME", t.TempDir())
	first, release, directory := installFixture(t)
	var err error
	first.Cache, err = NewCache("")
	require.NoError(t, err)
	layout := filepath.Join(t.TempDir(), "layout")
	require.NoError(t, pluginoci.Pack(directory, layout, first.Descriptors))
	request := &Request{Source: "oci://ghcr.io/acme/plugin", Constraint: "^1", Layout: layout}
	entry, err := first.Prepare(t.Context(), request)
	require.NoError(t, err)
	firstPath, err := first.Cache.Installed(entry, CurrentPlatform())
	require.NoError(t, err)
	second, err := NewService(&schema.AtmosConfiguration{}, first.Descriptors)
	require.NoError(t, err)
	second.Registry = NewMockRegistry(gomock.NewController(t))
	second.Config.Plugins.Policy.Offline = true
	restored, err := second.Prepare(t.Context(), &Request{Source: entry.Source, Constraint: entry.Constraint, Locked: entry})
	require.NoError(t, err)
	secondPath, err := second.Cache.Installed(restored, CurrentPlatform())
	require.NoError(t, err)
	require.Equal(t, firstPath, secondPath)
	require.Equal(t, release.Digest, restored.IndexDigest)
}

func TestServiceInitializationRejectsInvalidConfigurationAndIdentity(t *testing.T) {
	for _, test := range []struct {
		name   string
		config schema.PluginsConfig
		want   error
	}{
		{"invalid concurrency", schema.PluginsConfig{MaxConcurrency: -1}, errUtils.ErrPluginInvalidConfig},
		{"unknown registry identity", schema.PluginsConfig{Registries: map[string]schema.PluginRegistry{
			"private": {Host: "registry.example.com", Identity: "missing"},
		}}, errUtils.ErrIdentityNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := &schema.AtmosConfiguration{Plugins: test.config}
			config.Plugins.CacheDir = t.TempDir()
			service, err := NewService(config, extension.New())
			require.ErrorIs(t, err, test.want)
			require.Nil(t, service, "invalid configuration must not yield an operational installer")
		})
	}
}

func TestServiceInitializationRejectsUnusableDefaultCache(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "data")
	require.NoError(t, os.WriteFile(blocked, []byte("preserve"), 0o600))
	t.Setenv("ATMOS_XDG_DATA_HOME", "")
	t.Setenv("XDG_DATA_HOME", blocked)
	service, err := NewService(&schema.AtmosConfiguration{}, extension.New())
	require.Error(t, err)
	require.Nil(t, service)
	value, err := os.ReadFile(blocked)
	require.NoError(t, err)
	require.Equal(t, "preserve", string(value), "cache initialization must not overwrite an existing file")
}

func TestSignedMultiPlatformReleaseSharesLockAndSelectsIsolatedPayloads(t *testing.T) {
	service, _, directory := installFixture(t)
	platforms := []string{"linux/amd64", "linux/arm64", "linux/arm/v6", "linux/arm/v7"}
	raw := `apiVersion: atmos/v1
kind: AtmosPluginConfig
metadata: {name: acme/fixture, version: 1.0.0}
spec:
  runtime: {type: native, protocol: 1, executable: bin/plugin}
  compatibility: {protocol: ">=1 <2"}
  capabilities: {fixtures: {acme/fixture: {}}}
  platforms:
`
	for _, platform := range platforms {
		path := "bin/" + strings.ReplaceAll(platform, "/", "-")
		raw += fmt.Sprintf("    %s: {executable: %s}\n", platform, path)
		require.NoError(t, os.WriteFile(filepath.Join(directory, path), []byte("payload for "+platform), 0o755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(raw), 0o600))
	index, err := pluginoci.BuildIndex(directory, service.Descriptors)
	require.NoError(t, err)
	release, err := pluginoci.Inspect(index, "")
	require.NoError(t, err)
	layout := filepath.Join(t.TempDir(), "layout")
	require.NoError(t, pluginoci.Pack(directory, layout, service.Descriptors))
	authority := signaturetest.New(t)
	identity, issuer := "builder@example.com", "https://issuer.example.com"
	indexBytes, err := index.RawManifest()
	require.NoError(t, err)
	evidence := authority.Sign(t, identity, issuer, indexBytes)
	require.NoError(t, signature.Save(layout, [][]byte{evidence}))
	layoutBundles, err := signature.Load(layout)
	require.NoError(t, err)
	service.Config.Plugins.Policy = schema.PluginPolicy{
		Offline: true, RequireSignature: true,
		TrustedPublishers: []schema.PluginPublisher{{Publisher: "acme", Identity: identity, Issuer: issuer}},
	}
	service.Verifier.LoadRoot = func(context.Context, bool) (root.TrustedMaterial, error) { return authority, nil }
	service.Registry = NewMockRegistry(gomock.NewController(t))
	service.Platform = "linux/arm/v7"
	request := &Request{Source: "oci://ghcr.io/acme/fixture", Constraint: "1.0.0", Layout: layout}
	locked, err := service.Prepare(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, release.Digest, locked.IndexDigest)
	wantDigests := make(map[string]string)
	for _, platform := range platforms {
		wantDigests[platform] = release.Platforms[platform].Digest.String()
	}
	require.Equal(t, wantDigests, locked.Platforms, "a shared lock must retain every platform's digest")
	lockedBytes, err := json.Marshal(locked)
	require.NoError(t, err)
	request.Locked, request.Frozen = locked, true
	cachePaths := make(map[string]string)
	for _, test := range []struct{ requested, selected string }{
		{"linux/amd64/v2", "linux/amd64"},
		{"linux/arm64/v8", "linux/arm64"},
		{"linux/arm/v6", "linux/arm/v6"},
		{"linux/arm/v7", "linux/arm/v7"},
	} {
		t.Run(test.requested, func(t *testing.T) {
			service.Platform = test.requested
			request.Layout = layout
			entry, err := service.Prepare(t.Context(), request)
			require.NoError(t, err, "a frozen lock must install the compatible platform from an offline release")
			require.Equal(t, locked, entry, "switching architectures must not rewrite a shared lock")
			payload, err := service.Cache.Installed(entry, test.requested)
			require.NoError(t, err)
			cachePaths[test.selected] = payload
			wantPath, err := service.Cache.location(entry.IndexDigest, test.selected)
			require.NoError(t, err)
			require.Equal(t, filepath.Join(wantPath, payloadDirectory), payload)
			actual, err := os.ReadFile(filepath.Join(payload, "bin", "plugin"))
			require.NoError(t, err)
			require.Equal(t, "payload for "+test.selected, string(actual))
			bundles, err := signature.Load(wantPath)
			require.NoError(t, err)
			require.Equal(t, layoutBundles, bundles, "the exact verified layout evidence belongs beside the selected payload")
			request.Layout = ""
			reused, err := service.Prepare(t.Context(), request)
			require.NoError(t, err, "frozen offline reuse must find both payload and signature evidence through the variant selection")
			require.Equal(t, locked, reused)
			reusedBytes, err := json.Marshal(reused)
			require.NoError(t, err)
			require.Equal(t, lockedBytes, reusedBytes, "the complete lock entry must remain unchanged across platforms")
		})
	}
	for first, firstPath := range cachePaths {
		for second, secondPath := range cachePaths {
			if first != second {
				require.NotEqual(t, firstPath, secondPath, "different platform payloads must not overwrite one another")
				require.NotEqual(t, locked.Platforms[first], locked.Platforms[second])
			}
		}
	}
	for _, unsupported := range []string{"linux/arm/v5", "linux/386", "windows/amd64"} {
		_, err := service.Cache.Store(t.Context(), release, unsupported, service.Descriptors)
		require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedPlatform)
		_, err = service.Cache.Installed(locked, unsupported)
		require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedPlatform)
		_, err = service.Cache.ReleaseDirectory(locked, unsupported)
		require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedPlatform)
	}
	service.Platform = "linux/amd64/v2"
	amd64Payload := cachePaths["linux/amd64"]
	require.NoError(t, os.WriteFile(filepath.Join(amd64Payload, "bin", "plugin"), []byte("tampered"), 0o755))
	_, err = service.Prepare(t.Context(), request)
	require.ErrorIs(t, err, errUtils.ErrPluginCacheCorrupt)
	require.Contains(t, strings.Join(errUtils.AllHints(err), "\n"), filepath.Dir(amd64Payload), "repair guidance must identify the selected cache rather than the requested variant")
	require.Empty(t, quarantines(t, amd64Payload), "frozen reuse must preserve corrupt evidence")
	service.Platform = "linux/arm64/v8"
	intact, err := service.Prepare(t.Context(), request)
	require.NoError(t, err, "corruption of amd64 bytes must not affect arm64 installation or evidence")
	require.Equal(t, locked, intact)
}

func TestLockedUnsupportedPlatformFailsBeforeRegistryOrOfflineResolution(t *testing.T) {
	for _, offline := range []bool{false, true} {
		t.Run(fmt.Sprintf("offline=%t", offline), func(t *testing.T) {
			service, release, _ := installFixture(t)
			service.Config.Plugins.Policy.Offline = offline
			service.Platform = "unsupported/amd64"
			service.Registry = NewMockRegistry(gomock.NewController(t))
			locked := lockedEntry(release)
			locked.Source, locked.Constraint = "oci://ghcr.io/acme/fixture", "1.0.0"
			entry, err := service.Prepare(t.Context(), &Request{
				Source: locked.Source, Constraint: locked.Constraint, Locked: locked, Frozen: true,
			})
			require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedPlatform)
			require.NotErrorIs(t, err, errUtils.ErrPluginOfflineArtifact)
			require.Contains(t, err.Error(), service.Platform)
			require.Contains(t, err.Error(), CurrentPlatform())
			require.Nil(t, entry)
		})
	}
}
