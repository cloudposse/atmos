package autoinit

import (
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/terraform/clean"
)

// DataDir resolves the Terraform data directory the subprocess will actually use: lookup(TF_DATA_DIR)
// wins when it returns a non-empty value, falling back to os.Getenv(TF_DATA_DIR), and finally to
// the conventional ".terraform" default; lookup models the subprocess environment Atmos is about
// to launch terraform/tofu with and may be nil, in which case only os.Getenv is consulted. A
// relative result is joined to componentPath so callers always receive an absolute-ish path
// suitable for filesystem checks; the result is always filepath.Clean'ed.
func DataDir(componentPath string, lookup func(key string) string) string {
	defer perf.Track(nil, "autoinit.DataDir")()

	value := envLookup(lookup, clean.EnvTFDataDir)
	if value == "" {
		value = clean.TerraformDir
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(componentPath, value)
	}
	return filepath.Clean(value)
}

// MarkerPath returns the path of the init marker file inside dataDir.
func MarkerPath(dataDir string) string {
	defer perf.Track(nil, "autoinit.MarkerPath")()

	return filepath.Join(dataDir, MarkerFileName)
}

// envLookup consults lookup first (modeling a subprocess environment map that may not mirror the
// current process's own environment) and falls back to os.Getenv when lookup is nil or returns an
// empty value.
func envLookup(lookup func(key string) string, key string) string {
	if lookup != nil {
		if v := lookup(key); v != "" {
			return v
		}
	}
	//nolint:forbidigo // key is a Terraform env var (e.g. TF_DATA_DIR), not an Atmos config var.
	return os.Getenv(key)
}

// effectiveDataDir returns in.DataDir when explicitly set, otherwise derives it from
// in.ComponentPath and in.EnvLookup via DataDir.
func (in *Inputs) effectiveDataDir() string {
	if in.DataDir != "" {
		return in.DataDir
	}
	return DataDir(in.ComponentPath, in.EnvLookup)
}
