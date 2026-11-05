// Package config implements typed configuration records, inline schema helpers,
// JSON Schema loading, and immutable deep merging for automation scripts.
package config

import (
	"net/http"

	"go.starlark.net/starlark"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Host supplies runtime services without exposing the automation session.
// Callbacks and HTTP transports must be safe for concurrent use.
type Host struct {
	// ReadFile supplies configuration and local schema contents.
	ReadFile func(string) ([]byte, error)
	// ResolvePath resolves a path relative to the calling Starlark file.
	ResolvePath func(*starlark.Thread, string) string
	// HTTPClient optionally supplies the transport and a shorter request timeout.
	// The binding copies the client and enforces its own HTTPS redirect policy.
	HTTPClient *http.Client
}

type binding struct {
	host   Host
	client *http.Client
}

// New constructs the config namespace using the host's file and path services.
// Cancellation comes from the context attached to each calling thread.
func New(host Host) starlark.Value {
	defer perf.Track(nil, "config.New")()
	client := newSchemaHTTPClient()
	if host.HTTPClient != nil {
		*client = *host.HTTPClient
		client.CheckRedirect = newSchemaHTTPClient().CheckRedirect
		if client.Timeout <= 0 || client.Timeout > configSchemaTimeout {
			client.Timeout = configSchemaTimeout
		}
	}
	return (&binding{host: host, client: client}).configModule()
}
