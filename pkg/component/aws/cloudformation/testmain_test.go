package cloudformation

import (
	"context"
	"os"
	"testing"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/provisioner/backend"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// TestMain initializes the data and UI output layers for the package's tests.
// The default I/O context resolves os.Stdout/os.Stderr dynamically at write time,
// so tests that capture output by swapping os.Stdout continue to work.
//
// It also lets this package's own test binary stand in for a cross-platform
// "write a marker file" command, used by hooks: fixtures that need to prove a
// hook actually ran (e.g. TestRunWithHooks_SetsFailureOutcome) without
// depending on platform-specific binaries like `touch`. Mirrors
// cmd/helmfile/testmain_test.go.
func TestMain(m *testing.M) {
	if path := os.Getenv("_ATMOS_TEST_WRITE_MARKER"); path != "" {
		if err := os.WriteFile(path, []byte("after-hook-fired"), 0o600); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Packaging pre-checks the bucket before every upload. Default to "exists" so tests that
	// are not about the bucket never resolve real AWS credentials or probe the network; tests
	// that exercise the check call useRealBucketExistenceCheck.
	s3BucketExistsFunc = func(context.Context, *schema.AtmosConfiguration, map[string]any, *schema.AuthContext) (bool, error) {
		return true, nil
	}

	ioCtx, err := iolib.NewContext()
	if err != nil {
		panic(err)
	}
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	os.Exit(m.Run())
}

// useRealBucketExistenceCheck restores the production bucket existence probe for one test, so
// the S3 client factory installed by that test is the one that answers.
func useRealBucketExistenceCheck(t *testing.T) {
	t.Helper()
	original := s3BucketExistsFunc
	s3BucketExistsFunc = backend.S3BackendExists
	t.Cleanup(func() { s3BucketExistsFunc = original })
}
