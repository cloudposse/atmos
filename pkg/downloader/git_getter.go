package downloader

import (
	"net/url"

	"github.com/hashicorp/go-getter"

	"github.com/cloudposse/atmos/pkg/security"
)

// CustomGitGetter is a custom getter for git (git::) that validates symlinks based on security policy.
type CustomGitGetter struct {
	getter.GitGetter
	// Policy defines how symlinks should be handled. If not set, defaults to PolicyAllowSafe.
	Policy security.SymlinkPolicy
}

// Get implements the custom getter logic with symlink validation.
func (c *CustomGitGetter) Get(dst string, url *url.URL) error {
	// Normal clone
	if err := c.GetCustom(dst, url); err != nil {
		return err
	}

	// Validate symlinks based on policy (default to allow_safe if not configured)
	policy := c.Policy
	if policy == "" {
		policy = security.PolicyAllowSafe
	}

	return security.ValidateSymlinks(dst, policy)
}

// removeSymlinks walks the directory and removes any symlinks it encounters.
// Deprecated: Use security.ValidateSymlinks instead.
func removeSymlinks(root string) error {
	return security.ValidateSymlinks(root, security.PolicyRejectAll)
}
