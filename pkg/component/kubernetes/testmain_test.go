package kubernetes

import (
	"os"
	"testing"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

// TestMain initializes the data and UI output layers for the package's tests.
// The default I/O context resolves os.Stdout/os.Stderr dynamically at write time,
// so tests that capture output by swapping os.Stdout continue to work.
func TestMain(m *testing.M) {
	ioCtx, err := iolib.NewContext()
	if err != nil {
		panic(err)
	}
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	// envtestSetup is a no-op for the default (fast, fake-client) test tier. When
	// the package is built with `-tags envtest` it provisions the envtest
	// control-plane binaries (kube-apiserver+etcd) via the Atmos toolchain and
	// starts a real API server for the end-to-end tier. It returns a teardown
	// func that is invoked after the tests run (before os.Exit, which would skip
	// any deferred cleanup).
	teardownEnvtest := envtestSetup()
	code := m.Run()
	teardownEnvtest()

	os.Exit(code)
}
