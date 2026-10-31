package s3test

import (
	"os"
	"testing"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Environment variables that opt tests in to a real S3-compatible endpoint (Floci, MinIO, AWS).
// Credentials come from the usual AWS_* variables and the bucket must already exist.
const (
	EmulatorEndpointEnv = "ATMOS_BLOB_S3_ENDPOINT"
	EmulatorBucketEnv   = "ATMOS_BLOB_S3_BUCKET"
	EmulatorRegionEnv   = "ATMOS_BLOB_S3_REGION"
)

// Emulator describes a real S3-compatible endpoint chosen through the environment.
type Emulator struct {
	Endpoint string
	Bucket   string
	Region   string
}

// lookup returns the value of an opt-in test variable, or empty when it is unset.
func lookup(name string) string {
	value, _ := os.LookupEnv(name)
	return value
}

// EmulatorFromEnv returns the opt-in emulator, or skips the test when the environment does not
// name one. Emulator-backed tests supplement the fake and must never be required by ordinary runs.
func EmulatorFromEnv(t testing.TB) Emulator {
	defer perf.Track(nil, "s3test.EmulatorFromEnv")()

	t.Helper()
	emulator := Emulator{
		Endpoint: lookup(EmulatorEndpointEnv),
		Bucket:   lookup(EmulatorBucketEnv),
		Region:   lookup(EmulatorRegionEnv),
	}
	if emulator.Endpoint == "" || emulator.Bucket == "" {
		t.Skipf("set %s and %s to run against an S3-compatible endpoint", EmulatorEndpointEnv, EmulatorBucketEnv)
	}
	if emulator.Region == "" {
		emulator.Region = "us-east-1"
	}
	return emulator
}
