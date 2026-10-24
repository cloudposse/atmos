package version

import "github.com/cloudposse/atmos/internal/releaseidentity"

// Version holds the current version of the Atmos CLI binary.
// It can be set dynamically during build time using ldflags.
var Version = "test" // Default version; will be overridden during build

// Commit holds the full 40-character git commit SHA the binary was built
// from. It is set dynamically during build time using ldflags and is empty
// when the binary was built without them (e.g. `go run`/`go install`).
var Commit = ""

func init() {
	if releaseidentity.Value == "" {
		return // Preserve development builds and existing Version/Commit ldflags.
	}
	var err error
	Version, Commit, err = releaseidentity.Parse(releaseidentity.Value)
	if err != nil {
		panic(err)
	}
}
