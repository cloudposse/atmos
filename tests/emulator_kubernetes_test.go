package tests

import (
	"testing"
	"time"
)

// TestEmulatorKubernetesLifecycle exercises `atmos emulator up kubernetes` (k3s)
// end to end: atmos manages the privileged container and the Kubernetes API server
// accepts connections on the published port. This complements the docker-compose-based
// demo-helmfile example by covering the emulator-component lifecycle.
//
// Running k3s starts a nested Kubernetes, so it needs longer than the cloud-API emulators
// to become ready. Opt-in and runtime-gated like the other emulator E2Es.
func TestEmulatorKubernetesLifecycle(t *testing.T) {
	runEmulatorTCPLifecycle(t, "emulator-kubernetes", "kubernetes", "16443", 3*time.Minute)
}
