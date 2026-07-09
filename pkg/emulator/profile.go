package emulator

import (
	"fmt"
	"sort"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Endpoint is the live connection info for a running emulator container,
// resolved from the runtime's reported port bindings.
type Endpoint struct {
	// Target is what the emulator emulates: aws|gcp|azure|kubernetes|vault|registry.
	Target string
	// Host is the host the emulator is reachable on (typically "localhost").
	Host string
	// Ports maps a container port to its live host port.
	Ports map[int]int
	// NetworkIPs maps container network names to the container IP on that network.
	NetworkIPs map[string]string
	// Region is the cloud region (aws/gcp/azure), when configured.
	Region string
	// Project is the GCP project, when configured.
	Project string
	// Services are the enabled emulated services (informational; drives env/endpoints).
	Services []string
}

// HostPort returns the live host port bound to the given container port, and
// whether a binding exists.
func (e *Endpoint) HostPort(containerPort int) (int, bool) {
	defer perf.Track(nil, "emulator.Endpoint.HostPort")()

	port, ok := e.Ports[containerPort]
	return port, ok
}

// PrimaryHostPort returns the live host port bound to the lowest-numbered
// container port — the conventional "primary" endpoint for single-port emulators
// (e.g. Floci/LocalStack on 4566).
func (e *Endpoint) PrimaryHostPort() (int, bool) {
	defer perf.Track(nil, "emulator.Endpoint.PrimaryHostPort")()

	if len(e.Ports) == 0 {
		return 0, false
	}
	containerPorts := make([]int, 0, len(e.Ports))
	for cp := range e.Ports {
		containerPorts = append(containerPorts, cp)
	}
	sort.Ints(containerPorts)
	return e.Ports[containerPorts[0]], true
}

// NetworkIP returns the first container-network IP in stable network-name order.
func (e *Endpoint) NetworkIP() (string, bool) {
	defer perf.Track(nil, "emulator.Endpoint.NetworkIP")()

	if len(e.NetworkIPs) == 0 {
		return "", false
	}
	networks := make([]string, 0, len(e.NetworkIPs))
	for network := range e.NetworkIPs {
		networks = append(networks, network)
	}
	sort.Strings(networks)
	for _, network := range networks {
		if ip := e.NetworkIPs[network]; ip != "" {
			return ip, true
		}
	}
	return "", false
}

// loopbackHostToIPv4 rewrites a loopback or wildcard host to the IPv4 loopback
// literal 127.0.0.1, leaving real hostnames untouched. We connect to a host port
// that a container runtime published for the emulator; on Linux, Docker publishes
// that port on IPv4 0.0.0.0 only, while "localhost" resolves to IPv6 ::1 first.
// A connection to ::1 against an IPv4-only published port does not refuse — it
// hangs — so clients (e.g. Terraform's AWS SDK) stall until they time out. This
// bites GitHub Actions Linux runners in particular; locally Docker Desktop also
// binds [::], which masks it. 127.0.0.1 reaches a published port on every
// platform, so normalizing loopback to it is always safe. An empty host means
// "not set", which we also treat as loopback.
func loopbackHostToIPv4(host string) string {
	switch host {
	case "", "localhost", "::1", "[::1]", "::", "[::]", "0.0.0.0":
		return "127.0.0.1"
	default:
		return host
	}
}

// URL builds a scheme://host:port URL for the primary host port. Returns "" when
// no port is bound.
func (e *Endpoint) URL(scheme string) string {
	defer perf.Track(nil, "emulator.Endpoint.URL")()

	port, ok := e.PrimaryHostPort()
	if !ok {
		return ""
	}
	return fmt.Sprintf("%s://%s:%d", scheme, loopbackHostToIPv4(e.Host), port)
}

// Authority returns the live "host:port" (no scheme) for the primary host port,
// or "" when no port is bound. Used by targets whose env vars want a bare
// host:port (GCP emulator hosts, Azure endpoints, registries).
func (e *Endpoint) Authority() string {
	defer perf.Track(nil, "emulator.Endpoint.Authority")()

	port, ok := e.PrimaryHostPort()
	if !ok {
		return ""
	}
	return fmt.Sprintf("%s:%d", loopbackHostToIPv4(e.Host), port)
}

// Profile is what a driver advertises for a live emulator. Consumers subscribe
// to the parts they need: auth identities take Env (and, for kubernetes,
// Kubeconfig); the AWS resolver takes ResolverURL; the provider-config
// contributor takes Provider.
type Profile struct {
	// Env is the SDK / Terraform / VAULT_ADDR / registry environment.
	Env map[string]string
	// Kubeconfig holds a materialized kubeconfig for kubernetes targets.
	Kubeconfig []byte
	// ResolverURL is the base endpoint for Atmos's internal AWS SDK (aws only).
	ResolverURL string
	// Provider is a Terraform provider-config fragment (endpoints + skip-flags +
	// creds) consumed by the provider-config contributor.
	Provider map[string]any
}
