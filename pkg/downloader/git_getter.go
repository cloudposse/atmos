package downloader

import (
	"net/url"
	"os"
	"path/filepath"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/hashicorp/go-getter"
)

// CustomGitGetter is a custom getter for git (git::) that removes symlinks.
// It also supports retry configuration for transient errors.
type CustomGitGetter struct {
	getter.GitGetter
	// RetryConfig specifies retry behavior for git operations.
	RetryConfig *schema.RetryConfig
}

// Get implements the custom getter logic removing symlinks.
func (c *CustomGitGetter) Get(dst string, url *url.URL) error {
	// Normal clone
	if err := c.GetCustom(dst, url); err != nil {
		return err
	}
	// Remove symlinks
	return removeSymlinks(dst)
}

// removeSymlinks walks the directory and removes any symlinks it encounters.
func removeSymlinks(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			log.Debug("Removing symlink", "path", path)
			// Symlinks are removed for the entire repo, regardless if there are any subfolders specified
			return os.Remove(path)
		}
		return nil
	})
}
