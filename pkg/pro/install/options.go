// Package install provides the Atmos Pro installation service that scaffolds
// GitHub Actions workflows, auth profiles, and stack configuration.
package install

// OnConflictFunc is called when a file already exists.
// It receives the relative path and returns true to overwrite, false to skip.
// If it returns an error, installation is aborted.
type OnConflictFunc func(relPath string) (overwrite bool, err error)

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

	// OnConflict is called when a file exists and Force is false.
	// If nil, existing files are skipped silently.
	OnConflict OnConflictFunc
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

// WithOnConflict sets the callback for handling file conflicts.
func WithOnConflict(fn OnConflictFunc) Option {
	return func(o *Options) {
		o.OnConflict = fn
	}
}
