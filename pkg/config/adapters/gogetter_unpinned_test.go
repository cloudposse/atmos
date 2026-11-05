package adapters

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	log "github.com/cloudposse/atmos/pkg/logger"
)

func TestIsGitBackedImport(t *testing.T) {
	tests := []struct {
		importPath string
		want       bool
	}{
		{"git::https://github.com/org/repo.git//cfg.yaml?ref=v1.0.0", true},
		{"git@github.com:org/repo.git", true},
		{"github.com/org/repo//cfg.yaml", true},
		{"bitbucket.org/org/repo//cfg.yaml", true},
		{"https://raw.githubusercontent.com/org/repo/main/cfg.yaml", false},
		{"catalog/defaults.yaml", false},
		{"s3::https://s3.amazonaws.com/bucket/cfg.yaml", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, isGitBackedImport(tt.importPath), tt.importPath)
	}
}

func TestExtractRef(t *testing.T) {
	tests := []struct {
		importPath string
		want       string
	}{
		{"git::https://github.com/org/repo.git//cfg.yaml?ref=v1.2.3", "v1.2.3"},
		{"git::https://github.com/org/repo.git//cfg.yaml?ref=main&depth=1", "main"},
		{"git::https://github.com/org/repo.git//cfg.yaml", ""},
		{"github.com/org/repo//cfg.yaml?ref=abc1234", "abc1234"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, extractRef(tt.importPath), tt.importPath)
	}
}

func TestIsPinnedRef(t *testing.T) {
	pinned := []string{
		"v1.2.3",
		"1.2",
		"v2",
		"abc1234", // 7-char short SHA.
		"0123456789abcdef0123456789abcdef01234567", // 40-char SHA.
	}
	for _, ref := range pinned {
		assert.True(t, isPinnedRef(ref), "ref %q should be pinned", ref)
	}

	unpinned := []string{
		"",       // no ref.
		"main",   // default branch.
		"master", // default branch.
		"HEAD",
		"develop",
		"feature-x",    // branch.
		"release-2024", // non-version tag.
	}
	for _, ref := range unpinned {
		assert.False(t, isPinnedRef(ref), "ref %q should be unpinned", ref)
	}
}

func TestWarnIfUnpinnedRef(t *testing.T) {
	capture := func(importPath string) string {
		orig := log.Default()
		buf := &bytes.Buffer{}
		l := log.New()
		l.SetOutput(buf)
		l.SetLevel(log.WarnLevel)
		l.SetReportTimestamp(false)
		log.SetDefault(l)
		defer log.SetDefault(orig)

		warnIfUnpinnedRef(importPath)
		return buf.String()
	}

	t.Run("warns on unpinned git import (branch ref)", func(t *testing.T) {
		out := capture("git::https://github.com/org/repo.git//cfg.yaml?ref=main")
		assert.Contains(t, out, "not pinned")
	})

	t.Run("warns on git import with no ref", func(t *testing.T) {
		out := capture("git::https://github.com/org/repo.git//cfg.yaml")
		assert.Contains(t, out, "not pinned")
	})

	t.Run("no warning on SHA-pinned git import", func(t *testing.T) {
		out := capture("git::https://github.com/org/repo.git//cfg.yaml?ref=0123456789abcdef0123456789abcdef01234567")
		assert.Empty(t, strings.TrimSpace(out))
	})

	t.Run("no warning on version-tag-pinned git import", func(t *testing.T) {
		out := capture("git::https://github.com/org/repo.git//cfg.yaml?ref=v1.2.3")
		assert.Empty(t, strings.TrimSpace(out))
	})

	t.Run("no warning on non-git import", func(t *testing.T) {
		out := capture("https://raw.githubusercontent.com/org/repo/main/cfg.yaml")
		assert.Empty(t, strings.TrimSpace(out))
	})
}
