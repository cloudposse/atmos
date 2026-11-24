package source

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestFloatingRef(t *testing.T) {
	tests := []struct {
		name     string
		spec     schema.VendorComponentSource
		wantRef  string
		floating bool
	}{
		{name: "branch in query", spec: schema.VendorComponentSource{Uri: "git::https://example.invalid/m.git?ref=main"}, wantRef: "main", floating: true},
		{name: "branch with subdirectory", spec: schema.VendorComponentSource{Uri: "git::file:///repo//sub?ref=feature/x"}, wantRef: "feature/x", floating: true},
		{name: "branch via version", spec: schema.VendorComponentSource{Uri: "github.com/org/repo//m", Version: "develop"}, wantRef: "develop", floating: true},
		{name: "heads ref", spec: schema.VendorComponentSource{Uri: "git::https://example.invalid/m.git?ref=refs/heads/main"}, wantRef: "refs/heads/main", floating: true},
		{name: "semver tag", spec: schema.VendorComponentSource{Uri: "git::https://example.invalid/m.git?ref=v1.2.3"}},
		{name: "bare version", spec: schema.VendorComponentSource{Uri: "git::https://example.invalid/m.git?ref=1.2"}},
		{name: "prerelease tag", spec: schema.VendorComponentSource{Uri: "git::https://example.invalid/m.git?ref=v1.2.3-rc.1"}},
		{name: "tag ref", spec: schema.VendorComponentSource{Uri: "git::https://example.invalid/m.git?ref=refs/tags/v2"}},
		{name: "commit", spec: schema.VendorComponentSource{Uri: "git::https://example.invalid/m.git?ref=0123456789abcdef0123456789abcdef01234567"}},
		{name: "short commit", spec: schema.VendorComponentSource{Uri: "git::https://example.invalid/m.git?ref=abc1234"}},
		{name: "query wins over version", spec: schema.VendorComponentSource{Uri: "git::https://example.invalid/m.git?ref=v1", Version: "main"}},
		{name: "git source without a ref follows its default branch", spec: schema.VendorComponentSource{Uri: "git::https://example.invalid/m.git"}, floating: true},
		{name: "github path without a ref", spec: schema.VendorComponentSource{Uri: "github.com/org/repo//m"}, floating: true},
		{name: "git repository with subdirectory", spec: schema.VendorComponentSource{Uri: "https://example.invalid/m.git//sub"}, floating: true},
		{name: "archive without a ref", spec: schema.VendorComponentSource{Uri: "https://example.invalid/m.tar.gz"}},
		{name: "object store without a ref", spec: schema.VendorComponentSource{Uri: "s3::https://s3.amazonaws.com/bucket/m.zip"}},
		{name: "local path", spec: schema.VendorComponentSource{Uri: "./modules/m", Version: "main"}},
		{name: "empty", spec: schema.VendorComponentSource{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ref, floating := floatingRef(&test.spec)
			assert.Equal(t, test.floating, floating)
			assert.Equal(t, test.wantRef, ref)
		})
	}
	_, floating := floatingRef(nil)
	assert.False(t, floating)
}

func TestFloatingRefNotice(t *testing.T) {
	spec := &schema.VendorComponentSource{Uri: "git::https://example.invalid/m.git?ref=main"}
	notice := floatingRefNotice(spec, "terraform", "vpc", "live/prod")
	assert.Contains(t, notice, `floating ref "main"`)
	assert.Contains(t, notice, "atmos terraform source pull vpc -s live/prod --force")

	assert.Contains(t, floatingRefNotice(spec, "", "vpc", ""), "atmos terraform source pull vpc --force")

	// The command matches the component's own type.
	assert.Contains(t, floatingRefNotice(spec, "helmfile", "ingress", "live/prod"), "atmos helmfile source pull ingress -s live/prod --force")
	assert.Contains(t, floatingRefNotice(spec, "aws/cloudformation", "stack", ""), "atmos aws cloudformation source pull stack --force")

	// A Git source without a ref follows its default branch.
	assert.Contains(t, floatingRefNotice(&schema.VendorComponentSource{Uri: "git::https://example.invalid/m.git"}, "terraform", "vpc", ""), "follows its default branch")

	// A TTL already decides freshness, and immutable refs cannot drift.
	withTTL := *spec
	withTTL.TTL = "1h"
	assert.Empty(t, floatingRefNotice(&withTTL, "terraform", "vpc", "live/prod"))
	assert.Empty(t, floatingRefNotice(&schema.VendorComponentSource{Uri: "git::https://example.invalid/m.git?ref=v1"}, "terraform", "vpc", "live/prod"))
}
