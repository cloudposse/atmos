package version

// Version holds the current version of the Atmos CLI.
// It can be set dynamically during build time using ldflags.
var Version = "0.0.1" // Default version; will be overridden during build
