package devcontainer

//go:generate go run go.uber.org/mock/mockgen@latest -source=runtime_interface.go -destination=mock_runtime_detector_test.go -package=devcontainer

import (
	"github.com/cloudposse/atmos/pkg/container"
)

// RuntimeDetector handles container runtime detection.
type RuntimeDetector interface {
	// DetectRuntime detects and returns the appropriate container runtime.
	DetectRuntime(preferred string) (container.Runtime, error)
}

// runtimeDetectorImpl implements RuntimeDetector using existing functions.
type runtimeDetectorImpl struct{}

// NewRuntimeDetector creates a new RuntimeDetector.
func NewRuntimeDetector() RuntimeDetector {
	return &runtimeDetectorImpl{}
}

// DetectRuntime detects and returns the appropriate container runtime.
func (r *runtimeDetectorImpl) DetectRuntime(preferred string) (container.Runtime, error) {
	return DetectRuntime(preferred)
}
