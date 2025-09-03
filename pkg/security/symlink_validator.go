package security

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	log "github.com/charmbracelet/log"
	cp "github.com/otiai10/copy"

	"github.com/cloudposse/atmos/pkg/schema"
)

// SymlinkPolicy defines how symlinks should be handled during vendoring operations.
type SymlinkPolicy string

const (
	// PolicyAllowSafe validates symlink targets stay within boundaries (default).
	PolicyAllowSafe SymlinkPolicy = "allow_safe"

	// PolicyRejectAll skips all symlinks for maximum security.
	PolicyRejectAll SymlinkPolicy = "reject_all"

	// PolicyAllowAll follows all symlinks without validation (legacy behavior).
	PolicyAllowAll SymlinkPolicy = "allow_all"

	// Log field constants.
	logFieldSrc      = "src"
	logFieldBoundary = "boundary"
)

// CreateSymlinkHandler creates an OnSymlink callback based on the security policy.
// The baseDir parameter defines the boundary that symlinks must not escape.
func CreateSymlinkHandler(baseDir string, policy SymlinkPolicy) func(string) cp.SymlinkAction {
	return func(src string) cp.SymlinkAction {
		switch policy {
		case PolicyRejectAll:
			log.Debug("Symlink rejected by policy", logFieldSrc, src, "policy", "reject_all")
			return cp.Skip

		case PolicyAllowAll:
			log.Debug("Symlink allowed without validation", logFieldSrc, src, "policy", "allow_all")
			return cp.Deep

		case PolicyAllowSafe:
			fallthrough
		default:
			if IsSymlinkSafe(src, baseDir) {
				log.Debug("Symlink validated and allowed", logFieldSrc, src)
				return cp.Deep
			}
			log.Warn("Symlink rejected - target outside boundary", logFieldSrc, src, logFieldBoundary, baseDir)
			return cp.Skip
		}
	}
}

// IsSymlinkSafe validates if a symlink target is within the specified boundary.
// Returns true if the symlink is safe to follow, false otherwise.
func IsSymlinkSafe(symlink, boundary string) bool {
	// Get the symlink target.
	target, err := os.Readlink(symlink)
	if err != nil {
		// If we can't read the symlink, consider it unsafe.
		log.Debug("Failed to read symlink", "symlink", symlink, "error", err)
		return false
	}

	// If target is relative, make it absolute relative to symlink's directory.
	// We need to evaluate it properly to resolve .. components.
	if !filepath.IsAbs(target) {
		target = filepath.Clean(filepath.Join(filepath.Dir(symlink), target))
	}

	// Make paths absolute for comparison.
	cleanTarget, err := filepath.Abs(target)
	if err != nil {
		log.Debug("Failed to make target absolute", "target", target, "error", err)
		return false
	}

	cleanBoundary, err := filepath.Abs(filepath.Clean(boundary))
	if err != nil {
		log.Debug("Failed to clean boundary path", "boundary", boundary, "error", err)
		return false
	}

	// Check if target is within boundary.
	rel, err := filepath.Rel(cleanBoundary, cleanTarget)
	if err != nil {
		log.Debug("Failed to calculate relative path", "target", cleanTarget, "boundary", cleanBoundary, "error", err)
		return false
	}

	// If relative path starts with ".." or is absolute, it's outside boundary.
	if strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		log.Debug("Symlink target outside boundary", "target", cleanTarget, logFieldBoundary, cleanBoundary, "rel", rel)
		return false
	}

	return true
}

// ValidateSymlinks walks a directory and validates or removes symlinks according to the policy.
// For PolicyRejectAll, all symlinks are removed.
// For PolicyAllowSafe, only unsafe symlinks are removed.
// For PolicyAllowAll, no symlinks are removed.
func ValidateSymlinks(root string, policy SymlinkPolicy) error {
	if policy == PolicyAllowAll {
		// Keep all symlinks (legacy behavior).
		return nil
	}

	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walking %s: %w", path, err)
		}

		// Check if this is a symlink.
		if info.Mode()&os.ModeSymlink == 0 {
			return nil
		}

		// Handle based on policy.
		switch policy {
		case PolicyRejectAll:
			// Remove all symlinks for maximum security.
			log.Debug("Removing symlink (reject_all policy)", "path", path)
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("removing symlink %s: %w", path, err)
			}
			return nil

		case PolicyAllowSafe:
			// Validate and remove unsafe symlinks.
			if !IsSymlinkSafe(path, root) {
				log.Warn("Removing unsafe symlink", "path", path)
				if err := os.Remove(path); err != nil {
					return fmt.Errorf("removing unsafe symlink %s: %w", path, err)
				}
			} else {
				log.Debug("Keeping safe symlink", "path", path)
			}
			return nil

		default:
			// For unknown policies, default to safe behavior.
			if !IsSymlinkSafe(path, root) {
				log.Warn("Removing unsafe symlink (unknown policy, defaulting to safe)", "path", path, "policy", policy)
				if err := os.Remove(path); err != nil {
					return fmt.Errorf("removing unsafe symlink %s (unknown policy %s): %w", path, policy, err)
				}
			}
			return nil
		}
	})
}

// ParsePolicy converts a string policy value to SymlinkPolicy type.
// Returns PolicyAllowSafe as default for unknown values.
func ParsePolicy(policy string) SymlinkPolicy {
	cleaned := strings.ToLower(strings.TrimSpace(policy))
	switch cleaned {
	case string(PolicyRejectAll), "reject-all":
		return PolicyRejectAll
	case string(PolicyAllowAll), "allow-all":
		return PolicyAllowAll
	case string(PolicyAllowSafe), "allow-safe", "":
		return PolicyAllowSafe
	default:
		log.Warn("Unknown symlink policy, defaulting to allow_safe", "policy", policy)
		return PolicyAllowSafe
	}
}

// GetPolicyFromConfig retrieves the symlink policy from the AtmosConfiguration.
// Returns PolicyAllowSafe as the default if not configured.
func GetPolicyFromConfig(atmosConfig *schema.AtmosConfiguration) SymlinkPolicy {
	if atmosConfig == nil {
		return PolicyAllowSafe
	}

	if atmosConfig.Vendor.Policy.Symlinks == "" {
		return PolicyAllowSafe
	}

	return ParsePolicy(atmosConfig.Vendor.Policy.Symlinks)
}
