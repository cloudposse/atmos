package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"text/template"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// keyTemplateData is the data made available to key templates.
type keyTemplateData struct {
	// OS is the GOOS value (e.g. "linux", "darwin").
	OS string
	// Arch is the GOARCH value (e.g. "amd64", "arm64").
	Arch string
}

// renderKey renders a cache key template. The template may use {{.OS}},
// {{.Arch}} and the hashFiles function, e.g.:
//
//	atmos-toolchain-{{.OS}}-{{.Arch}}-{{ hashFiles "toolchain.lock.yaml" }}
//
// hashFiles returns a stable short SHA256 over the contents of the matched
// files (relative to baseDir), or "no-files" when none exist — mirroring the
// behavior of the GitHub Actions hashFiles() expression closely enough for
// cache-key derivation.
func renderKey(tmpl, baseDir string) (string, error) {
	defer perf.Track(nil, "cache.renderKey")()

	funcs := template.FuncMap{
		"hashFiles": func(patterns ...string) string {
			return hashFiles(baseDir, patterns)
		},
	}

	t, err := template.New("cache-key").Funcs(funcs).Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("%w: invalid cache key template: %w", errUtils.ErrCacheInvalidArgs, err)
	}

	var sb strings.Builder
	data := keyTemplateData{OS: runtime.GOOS, Arch: runtime.GOARCH}
	if err := t.Execute(&sb, data); err != nil {
		return "", fmt.Errorf("%w: failed to render cache key: %w", errUtils.ErrCacheInvalidArgs, err)
	}

	key := strings.TrimSpace(sb.String())
	if key == "" {
		return "", errUtils.ErrCacheKeyRequired
	}
	return key, nil
}

// hashFiles computes a stable SHA256 over the contents of the files matched by
// the given glob patterns (resolved relative to baseDir), in sorted order.
// Returns a 16-char hex prefix, or "no-files" when nothing matches.
func hashFiles(baseDir string, patterns []string) string {
	if len(patterns) == 0 {
		return "no-files"
	}

	var matched []string
	for _, pattern := range patterns {
		full := pattern
		if !filepath.IsAbs(full) {
			full = filepath.Join(baseDir, pattern)
		}
		paths, err := filepath.Glob(full)
		if err != nil {
			continue
		}
		matched = append(matched, paths...)
	}
	if len(matched) == 0 {
		return "no-files"
	}

	sort.Strings(matched)
	h := sha256.New()
	hashedAny := false
	for _, p := range matched {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		_, _ = h.Write([]byte(filepath.Base(p)))
		_, _ = h.Write(data)
		hashedAny = true
	}
	if !hashedAny {
		return "no-files"
	}
	return hex.EncodeToString(h.Sum(nil))[:hashPrefixLen]
}

// DefaultKey returns the default cache key derived from the toolchain lockfile
// (if present under root), the OS/arch, and a stable namespace. The lockfilePath
// argument is the absolute path to toolchain.lock.yaml; when empty or missing, a
// stable "no-lock" marker is used so restore-key fallback still works.
func defaultKey(lockfilePath string) string {
	defer perf.Track(nil, "cache.defaultKey")()

	hash := "no-lock"
	if lockfilePath != "" {
		if data, err := os.ReadFile(lockfilePath); err == nil {
			sum := sha256.Sum256(data)
			hash = hex.EncodeToString(sum[:])[:hashPrefixLen]
		}
	}
	return fmt.Sprintf("%s%s-%s-%s", defaultKeyPrefix, runtime.GOOS, runtime.GOARCH, hash)
}

// defaultRestoreKey returns the prefix fallback used when the exact default key
// is absent (same namespace + OS/arch, without the lockfile hash).
func defaultRestoreKey() string {
	return fmt.Sprintf("%s%s-%s-", defaultKeyPrefix, runtime.GOOS, runtime.GOARCH)
}
