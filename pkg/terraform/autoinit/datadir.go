package autoinit

import (
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/terraform/clean"
)

// DataDir resolves the Terraform data directory the subprocess will actually use: lookup(TF_DATA_DIR)
// wins whenever it reports the key as present (its second return value), even when the value
// itself is an explicit empty string, falling back to os.Getenv(TF_DATA_DIR) only when lookup is
// nil or reports the key absent, and finally to the conventional ".terraform" default; lookup
// models the subprocess environment Atmos is about to launch terraform/tofu with. A relative
// result is joined to componentPath so callers always receive an absolute-ish path suitable for
// filesystem checks; the result is always filepath.Clean'ed.
func DataDir(componentPath string, lookup func(key string) (string, bool)) string {
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
// current process's own environment) and falls back to os.Getenv only when lookup is nil or
// reports the key absent (its second return value is false). An explicit empty-string value that
// lookup reports as present is honored as-is -- it is NOT treated the same as "absent" -- so a
// component override that clears an inherited env var (e.g. `env: {TF_CLI_CONFIG_FILE: ""}`) is
// distinguishable from that var simply not being configured at all, and the fingerprint reflects
// the exact value the subprocess will see rather than silently falling back to this process's own
// ambient environment.
func envLookup(lookup func(key string) (string, bool), key string) string {
	if lookup != nil {
		if v, ok := lookup(key); ok {
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
