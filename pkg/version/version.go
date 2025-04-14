package version

// Version holds the current version of the Atmos CLI.
// It can be set dynamically during build time using ldflags.
var Version = "test" // Default version; will be overridden during build
