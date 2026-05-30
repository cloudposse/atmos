package downloader

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsRemoteSource(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want bool
	}{
		{"https url", "https://github.com/acme/repo.git", true},
		{"git:: prefixed https", "git::https://github.com/acme/repo.git//modules/vpc", true},
		{"ssh url", "ssh://git@github.com/acme/repo.git", true},
		{"scp-style git url", "git@github.com:acme/repo.git", true},
		{"github shorthand", "github.com/acme/repo", true},
		{"github shorthand with subdir", "github.com/acme/repo//modules/vpc", true},
		{"gitlab shorthand", "gitlab.com/acme/repo", true},
		{"bitbucket shorthand", "bitbucket.org/acme/repo", true},

		{"explicit file scheme", "file:///tmp/local/path", false},
		{"absolute local path", "/tmp/local/components/vpc", false},
		{"relative local path", "./components/vpc", false},
		{"parent relative path", "../shared/module", false},
		{"unsupported host shorthand", "example.com/acme/repo", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isRemoteSource(tt.src))
		})
	}
}
