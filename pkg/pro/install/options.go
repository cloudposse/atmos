// Package install provides the Atmos Pro installation service that scaffolds
// GitHub Actions workflows, auth profiles, and stack configuration.
package install

// Options configures the installation behavior.
type Options struct {
	// BasePath is the project root directory (default: cwd).
	BasePath string

	// StacksBasePath is the stacks directory relative to BasePath (from atmos.yaml).
	StacksBasePath string

	// Force overwrites existing files instead of skipping them.
	Force bool

	// DryRun previews what would be created without writing files.
	DryRun bool
}

// Option is a functional option for configuring the installer.
type Option func(*Options)

// WithBasePath sets the project root directory.
func WithBasePath(path string) Option {
	return func(o *Options) {
		o.BasePath = path
	}
}

// WithStacksBasePath sets the stacks directory.
func WithStacksBasePath(path string) Option {
	return func(o *Options) {
		o.StacksBasePath = path
	}
}

// WithForce enables overwriting existing files.
func WithForce(force bool) Option {
	return func(o *Options) {
		o.Force = force
	}
}

// WithDryRun enables preview mode.
func WithDryRun(dryRun bool) Option {
	return func(o *Options) {
		o.DryRun = dryRun
	}
}
