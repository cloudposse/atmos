package autoinit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/hashfile"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/terraform/clean"
	"github.com/cloudposse/atmos/pkg/terraform/lockfile"
)

// Environment variable names read while computing a fingerprint.
const (
	envTFCLIArgs         = "TF_CLI_ARGS"
	envTFCLIArgsInit     = "TF_CLI_ARGS_init"
	envTFPluginCacheDir  = "TF_PLUGIN_CACHE_DIR"
	envTFCLIConfigFile   = "TF_CLI_CONFIG_FILE"
	envTofuCLIConfigFile = "TOFU_CLI_CONFIG_FILE"
)

// envRecordKeys are the environment variables whose current value is recorded verbatim as a
// fingerprint fact (see buildRecords).
var envRecordKeys = []string{envTFCLIArgs, envTFCLIArgsInit, envTFPluginCacheDir, clean.EnvTFDataDir}

// errWrapFingerprint wraps a lower-level error under ErrInitFingerprint so every fingerprinting
// failure -- regardless of which stage it originated in -- is identifiable via errors.Is.
const errWrapFingerprint = "%w: %w"

// rootConfigPatterns are the root-level (non-recursive) Terraform/OpenTofu configuration file
// globs considered part of the init fingerprint.
var rootConfigPatterns = []string{"*.tf", "*.tf.json", "*.tofu", "*.tofu.json"}

// backendHCLPattern matches an HCL `backend "..."` block header or a `cloud {` block, either of
// which changes what `terraform init` needs to do.
var backendHCLPattern = regexp.MustCompile(`(?m)^\s*backend\s+"|\bcloud\s*\{`)

// Inputs describes everything that determines whether a previously recorded init is still valid.
type Inputs struct {
	// ComponentPath is the absolute component (or workdir) directory init would run in.
	ComponentPath string
	// DataDir is the Terraform data directory; if empty it is computed with
	// DataDir(ComponentPath, EnvLookup).
	DataDir string
	// VarFile is the varfile path (absolute, or relative to ComponentPath); hashed only when
	// PassVars is true.
	VarFile string
	// PassVars indicates whether Atmos is passing a varfile (and TF_VAR_* extras) to terraform.
	PassVars bool
	// Binary is the resolved terraform/tofu executable. Bare names are resolved via
	// exec.LookPath; on failure only the name itself is recorded.
	Binary string
	// EnvLookup models the subprocess environment Atmos is about to launch terraform/tofu with.
	// Its second return value distinguishes "key present with this value" (honored as-is, even
	// when the value is "") from "key absent" (falls back to os.Getenv). A nil EnvLookup falls
	// back to os.Getenv unconditionally.
	EnvLookup func(key string) (string, bool)
	// Extra holds caller-supplied records (e.g. TF_VAR_* values when PassVars is true) that
	// should participate in the fingerprint.
	Extra map[string]string
}

// Fingerprint is the computed digest of a set of Inputs.
type Fingerprint struct {
	// Hash is the sha256 hex digest over every input that can affect `terraform init`.
	Hash string
	// BackendHash is the sha256 hex digest over only the backend-relevant subset of files.
	BackendHash string
	// Files lists the relative file names that contributed to Hash, sorted, for debug logs.
	Files []string
}

// Compute derives a Fingerprint from in. Every file that contributes to the digest is recorded by
// name relative to in.ComponentPath, never by absolute path, so two checkouts of the same
// component at different locations on disk produce identical fingerprints.
func Compute(in *Inputs) (Fingerprint, error) {
	defer perf.Track(nil, "autoinit.Compute")()

	files, err := collectFiles(in)
	if err != nil {
		return Fingerprint{}, fmt.Errorf(errWrapFingerprint, errUtils.ErrInitFingerprint, err)
	}

	records := buildRecords(in)

	hash, err := hashfile.HashNamedFiles(files.named, records)
	if err != nil {
		return Fingerprint{}, fmt.Errorf(errWrapFingerprint, errUtils.ErrInitFingerprint, err)
	}

	backendHash, err := hashfile.HashNamedFiles(files.backendNamed, []string{"schema=1"})
	if err != nil {
		return Fingerprint{}, fmt.Errorf(errWrapFingerprint, errUtils.ErrInitFingerprint, err)
	}

	sort.Strings(files.names)
	return Fingerprint{Hash: hash, BackendHash: backendHash, Files: files.names}, nil
}

// collectedFiles bundles collectFiles' results: named holds every file that contributes to the
// main fingerprint keyed by base name, backendNamed the narrower subset relevant only to the
// backend configuration, and names the (unsorted) base names contributing to named.
type collectedFiles struct {
	named, backendNamed map[string]string
	names               []string
}

// collectFiles gathers every file that contributes to the main fingerprint (named, names) and the
// narrower subset relevant only to the backend configuration (backendNamed).
func collectFiles(in *Inputs) (collectedFiles, error) {
	named := map[string]string{}
	backendNamed := map[string]string{}
	var names []string

	rootFiles, err := collectRootConfigFiles(in.ComponentPath)
	if err != nil {
		return collectedFiles{}, err
	}
	for _, f := range rootFiles {
		name := filepath.Base(f)
		named[name] = f
		names = append(names, name)
		if content, readErr := os.ReadFile(f); readErr == nil && isBackendConfigFile(name, string(content)) {
			backendNamed[name] = f
		}
	}

	lockPath := filepath.Join(in.ComponentPath, lockfile.Name)
	if fileExists(lockPath) {
		named[lockfile.Name] = lockPath
		names = append(names, lockfile.Name)
	}

	backendPath := filepath.Join(in.ComponentPath, clean.BackendConfigFile)
	if fileExists(backendPath) {
		backendNamed[clean.BackendConfigFile] = backendPath
	}

	if in.PassVars {
		varFiles, varErr := collectVarFiles(in)
		if varErr != nil {
			return collectedFiles{}, varErr
		}
		for name, path := range varFiles {
			named[name] = path
			names = append(names, name)
		}
	}

	return collectedFiles{named: named, backendNamed: backendNamed, names: names}, nil
}

// explicitVarFileKeyPrefix namespaces in.VarFile's fingerprint record key so it can never collide
// with a component-local *.tfvars file that happens to share the same base name (e.g. an external
// `-var-file ../shared/terraform.tfvars` and a component-local terraform.tfvars). Without this,
// collectVarFiles' map (keyed only by filepath.Base) would silently drop one of the two entries,
// and edits to the dropped file would never change the fingerprint. The prefix leads with a NUL
// byte, which no filesystem permits in a filename (unlike ":", which is valid on Unix and would
// let a component-local file named e.g. "explicit-var-file:terraform.tfvars" collide with this
// prefix), so this key can never collide with a real base name on any platform.
const explicitVarFileKeyPrefix = "\x00explicit-var-file:"

// collectVarFiles resolves in.VarFile (if set) plus any root *.tfvars / *.tfvars.json files,
// keyed by base name. The explicit var file is keyed separately (see explicitVarFileKeyPrefix)
// so it never collides with a component-local var file of the same base name.
func collectVarFiles(in *Inputs) (map[string]string, error) {
	files := map[string]string{}

	if in.VarFile != "" {
		path := in.VarFile
		if !filepath.IsAbs(path) {
			path = filepath.Join(in.ComponentPath, path)
		}
		if fileExists(path) {
			files[explicitVarFileKeyPrefix+filepath.Base(path)] = path
		}
	}

	for _, pattern := range []string{"*.tfvars", "*.tfvars.json"} {
		matches, err := filepath.Glob(filepath.Join(in.ComponentPath, pattern))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", pattern, err)
		}
		for _, m := range matches {
			files[filepath.Base(m)] = m
		}
	}
	return files, nil
}

// collectRootConfigFiles returns the root-level (non-recursive) Terraform/OpenTofu configuration
// files in dir, sorted by name.
func collectRootConfigFiles(dir string) ([]string, error) {
	var files []string
	for _, pattern := range rootConfigPatterns {
		matches, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", pattern, err)
		}
		files = append(files, matches...)
	}
	sort.Strings(files)
	return files, nil
}

// isBackendConfigFile reports whether a root configuration file (named name, with content)
// declares a backend or cloud block. JSON files are checked with a simple substring test since a
// full HCL-JSON walk is unnecessary for this purpose.
func isBackendConfigFile(name, content string) bool {
	if strings.HasSuffix(name, ".json") {
		return strings.Contains(content, `"backend"`) || strings.Contains(content, `"cloud"`)
	}
	return backendHCLPattern.MatchString(content)
}

// buildRecords assembles the string facts (schema version, resolved binary, pass_vars, tracked
// environment variables, caller-supplied extras, and CLI config file content) that participate in
// the fingerprint alongside the named files. Every fact here is computed in-process (env lookups,
// os.Stat, exec.LookPath) with failures already degraded to a fallback record rather than
// propagated, so this never fails.
func buildRecords(in *Inputs) []string {
	records := []string{
		"schema=1",
		binaryRecord(in.Binary),
		fmt.Sprintf("pass_vars=%t", in.PassVars),
	}

	for _, key := range envRecordKeys {
		records = append(records, fmt.Sprintf("env:%s=%s", key, envLookup(in.EnvLookup, key)))
	}

	for k, v := range in.Extra {
		records = append(records, fmt.Sprintf("extra:%s=%s", k, v))
	}

	if cliRecord := cliConfigRecord(in.EnvLookup); cliRecord != "" {
		records = append(records, cliRecord)
	}

	return records
}

// binaryRecord records enough about the resolved terraform/tofu executable to detect a version or
// installation change: its absolute path, size, and modification time. Bare names are resolved
// via exec.LookPath first; when resolution or stat fails, only the raw name is recorded so a
// fingerprint can still be computed even when the binary cannot currently be found.
func binaryRecord(binary string) string {
	resolved := binary
	if isBareName(binary) {
		p, err := exec.LookPath(binary)
		if err != nil {
			return "binary=" + binary
		}
		resolved = p
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "binary=" + binary
	}
	return fmt.Sprintf("binary=%s|%d|%d", resolved, info.Size(), info.ModTime().UnixNano())
}

// isBareName reports whether binary is a plain executable name with no path component (e.g.
// "terraform"), as opposed to an absolute or relative path.
func isBareName(binary string) bool {
	return binary != "" && filepath.Base(binary) == binary
}

// cliConfigRecord returns a fingerprint record over the *content* (never the path) of the
// configured CLI config file(s), since Atmos writes that file to a fresh temporary path on every
// run -- hashing the path would make the fingerprint change on every invocation regardless of
// whether the content actually changed. Both TF_CLI_CONFIG_FILE and TOFU_CLI_CONFIG_FILE are
// included (each under its own key) whenever set and readable, rather than only the first one
// found: which variable OpenTofu actually honors depends on its own precedence rules (it prefers
// TOFU_CLI_CONFIG_FILE over TF_CLI_CONFIG_FILE), and hashing only one risks missing a change to
// whichever file is actually in effect. Returns "" when neither variable is set or readable.
func cliConfigRecord(lookup func(key string) (string, bool)) string {
	var parts []string
	for _, key := range []string{envTFCLIConfigFile, envTofuCLIConfigFile} {
		path := envLookup(lookup, key)
		if path == "" {
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			log.Debug("autoinit: cli config file not readable, excluding from fingerprint", "path", path, "error", err)
			continue
		}
		parts = append(parts, fmt.Sprintf("%s:%s", key, string(content)))
	}
	if len(parts) == 0 {
		return ""
	}
	return "cli_config:" + strings.Join(parts, "|")
}

// fileExists reports whether path exists and is a regular file (not a directory).
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
