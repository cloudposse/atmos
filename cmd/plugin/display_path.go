package plugin

import "github.com/cloudposse/atmos/pkg/plugin/config"

// displayPath renders a filesystem path for a status message. Paths under the current directory
// are shown relative to it, because that is how the user addressed them; anything else stays
// absolute so it is never ambiguous.
func displayPath(path string) string { return config.DisplayPath(path) }
