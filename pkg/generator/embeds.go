package generator

import "embed"

// Templates contains embedded init/scaffold templates shipped with the binary.
// The `all:` prefix embeds dot- and underscore-prefixed files too (e.g.
// `.gitignore` and `stacks/_defaults.yaml`), which plain `//go:embed` skips.
//
//go:embed all:templates
var Templates embed.FS
