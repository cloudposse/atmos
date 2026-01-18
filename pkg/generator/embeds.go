package generator

import "embed"

// Templates contains embedded init/scaffold templates shipped with the binary.
//
//go:embed templates
var Templates embed.FS
