package discovery

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	pluginconfig "github.com/cloudposse/atmos/pkg/plugin/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDefaultDiscoverySources(t *testing.T) {
	tests := []struct {
		name       string
		disabled   bool
		registries map[string]schema.PluginRegistry
		selected   string
		want       []source
		invalid    bool
	}{
		{name: "implicit default", want: []source{{alias: "cloudposse", reference: pluginconfig.DefaultCatalog, kind: "catalog", optional: true}}},
		{name: "explicit selection", selected: "cloudposse", want: []source{{alias: "cloudposse", reference: pluginconfig.DefaultCatalog, kind: "catalog"}}},
		{name: "disabled", disabled: true, want: []source{}},
		{name: "disabled selection", disabled: true, selected: "cloudposse", invalid: true},
		{name: "host override removes catalog", registries: map[string]schema.PluginRegistry{"cloudposse": {Host: "example.com"}}, want: []source{}},
		{name: "explicit override survives disabled default", disabled: true, registries: map[string]schema.PluginRegistry{"cloudposse": {Catalog: "oci://example.com/catalog:stable"}}, want: []source{{alias: "cloudposse", reference: "oci://example.com/catalog:stable", kind: "catalog"}}},
		{name: "selected private registry excludes default", selected: "private", registries: map[string]schema.PluginRegistry{"private": {Repositories: []string{"oci://example.com/tools"}}}, want: []source{{alias: "private", reference: "oci://example.com/tools", kind: "repository"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &Service{Config: &schema.AtmosConfiguration{Plugins: schema.PluginsConfig{Registries: tt.registries}}}
			if tt.disabled {
				service.Config.Plugins.Discovery.DefaultRegistry = new(false)
			}
			actual, err := service.sources(tt.selected)
			if tt.invalid {
				require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, actual)
		})
	}
}

func TestDefaultCatalogSearchReportsRegistryErrors(t *testing.T) {
	registry := NewMockRegistry(gomock.NewController(t))
	reference, err := name.ParseReference(strings.TrimPrefix(pluginconfig.DefaultCatalog, "oci://"), name.StrictValidation)
	require.NoError(t, err)
	unavailable := errors.New("catalog unavailable")
	registry.EXPECT().Head(gomock.Any(), reference).Return(nil, unavailable)
	service := &Service{Config: &schema.AtmosConfiguration{Plugins: schema.PluginsConfig{CacheDir: t.TempDir()}}, Registry: registry}
	_, err = service.Search(t.Context(), Request{})
	require.ErrorIs(t, err, unavailable)
	require.Contains(t, err.Error(), "cloudposse")
}

func TestDisabledDefaultSearchNeedsNoRegistry(t *testing.T) {
	registry := NewMockRegistry(gomock.NewController(t)) // No transport method may be called.
	service := &Service{Config: &schema.AtmosConfiguration{Plugins: schema.PluginsConfig{Discovery: schema.PluginDiscoveryConfig{DefaultRegistry: new(false)}}}, Registry: registry}
	results, err := service.Search(t.Context(), Request{})
	require.NoError(t, err)
	require.Empty(t, results)
}

func TestUnavailableImplicitCatalogPreservesPrivateDiscovery(t *testing.T) {
	failures := []struct {
		name string
		err  error
	}{
		{name: "not found", err: &transport.Error{StatusCode: http.StatusNotFound}},
		{name: "service unavailable", err: &transport.Error{StatusCode: http.StatusServiceUnavailable}},
		{name: "rate limited", err: &transport.Error{StatusCode: http.StatusTooManyRequests}},
		{name: "request timeout", err: &transport.Error{StatusCode: http.StatusRequestTimeout}},
		{name: "connection refused", err: &url.Error{Op: "Head", Err: &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}}},
		{name: "network unreachable", err: &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ENETUNREACH}},
		{name: "host unreachable", err: &net.OpError{Op: "dial", Net: "tcp", Err: syscall.EHOSTUNREACH}},
		{name: "dial deadline", err: &net.OpError{Op: "dial", Net: "tcp", Err: os.ErrDeadlineExceeded}},
		{name: "connect timeout", err: &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ETIMEDOUT}},
		{name: "Windows network unreachable", err: &net.OpError{Op: "dial", Net: "tcp", Err: windowsNetworkUnreachable}},
		{name: "Windows host unreachable", err: &net.OpError{Op: "dial", Net: "tcp", Err: windowsHostUnreachable}},
		{name: "Windows connection refused", err: &net.OpError{Op: "dial", Net: "tcp", Err: windowsConnectionRefused}},
		{name: "Windows connect timeout", err: &net.OpError{Op: "dial", Net: "tcp", Err: windowsConnectionTimeout}},
		{name: "DNS unavailable", err: &net.DNSError{Err: "no such host", Name: "ghcr.io", IsNotFound: true}},
	}
	for _, failure := range failures {
		t.Run(failure.name, func(t *testing.T) {
			f := newFixture(t)
			f.publish(t, "1.0.0", "Private tools")
			expected, err := f.service.Search(t.Context(), Request{})
			require.NoError(t, err)
			require.Len(t, expected, 1)
			f.service.Config.Plugins.Discovery.DefaultRegistry = nil
			registry := NewMockRegistry(gomock.NewController(t))
			ref, err := name.ParseReference(strings.TrimPrefix(pluginconfig.DefaultCatalog, "oci://"))
			require.NoError(t, err)
			registry.EXPECT().Head(gomock.Any(), ref).Return(nil, failure.err).Times(2)
			f.service.Registry = registry
			for range 2 {
				actual, err := f.service.Search(t.Context(), Request{})
				require.NoError(t, err)
				require.Equal(t, expected, actual, "unavailable defaults must not suppress private results or be cached")
			}
		})
	}
}

func TestExplicitCatalogRemainsRequired(t *testing.T) {
	for _, selected := range []bool{false, true} {
		t.Run(map[bool]string{false: "configured", true: "selected"}[selected], func(t *testing.T) {
			registry := NewMockRegistry(gomock.NewController(t))
			unavailable := &transport.Error{StatusCode: http.StatusNotFound}
			registry.EXPECT().Head(gomock.Any(), gomock.Any()).Return(nil, unavailable)
			service := &Service{Config: &schema.AtmosConfiguration{Plugins: schema.PluginsConfig{CacheDir: t.TempDir()}}, Registry: registry}
			request := Request{}
			if selected {
				request.Registry = pluginconfig.DefaultRegistryAlias
			} else {
				service.Config.Plugins.Registries = map[string]schema.PluginRegistry{pluginconfig.DefaultRegistryAlias: {Catalog: pluginconfig.DefaultCatalog}}
			}
			_, err := service.Search(t.Context(), request)
			require.ErrorIs(t, err, unavailable)
		})
	}
}

func TestImplicitCatalogFailsClosedForSecurityAndContentErrors(t *testing.T) {
	for _, failure := range []error{
		&transport.Error{StatusCode: http.StatusUnauthorized},
		&transport.Error{StatusCode: http.StatusForbidden},
		&transport.Error{StatusCode: http.StatusBadRequest},
		&url.Error{Op: "Head", Err: x509.UnknownAuthorityError{}},
		&net.OpError{Op: "remote error", Net: "tcp", Err: x509.UnknownAuthorityError{}},
		&net.OpError{Op: "dial", Net: "tcp", Err: x509.UnknownAuthorityError{}},
		&net.OpError{Op: "dial", Net: "tcp", Err: &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}},
		&net.OpError{Op: "dial", Net: "tcp", Err: tls.RecordHeaderError{Msg: "invalid TLS record"}},
		&net.OpError{Op: "dial", Net: "tcp", Err: errors.New("TLS handshake failed")},
		&net.OpError{Op: "dial", Net: "tcp", Err: syscall.EACCES},
		errUtils.ErrOCIIntegrity,
		context.Canceled,
		context.DeadlineExceeded,
	} {
		t.Run(failure.Error(), func(t *testing.T) {
			registry := NewMockRegistry(gomock.NewController(t))
			registry.EXPECT().Head(gomock.Any(), gomock.Any()).Return(nil, failure)
			service := &Service{Config: &schema.AtmosConfiguration{Plugins: schema.PluginsConfig{CacheDir: t.TempDir()}}, Registry: registry}
			_, err := service.Search(t.Context(), Request{})
			require.ErrorIs(t, err, failure)
		})
	}
	t.Run("missing content after successful HEAD", func(t *testing.T) {
		registry := NewMockRegistry(gomock.NewController(t))
		registry.EXPECT().Head(gomock.Any(), gomock.Any()).Return(&v1.Descriptor{Digest: v1.Hash{Algorithm: "sha256", Hex: strings.Repeat("a", 64)}}, nil)
		missing := &transport.Error{StatusCode: http.StatusNotFound}
		registry.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, missing)
		service := &Service{Config: &schema.AtmosConfiguration{Plugins: schema.PluginsConfig{CacheDir: t.TempDir()}}, Registry: registry}
		_, err := service.Search(t.Context(), Request{})
		require.ErrorIs(t, err, missing)
	})
}
