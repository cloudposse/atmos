package vendor

// PullOptions configures vendor pull behavior.
type PullOptions struct {
	DryRun        bool
	Component     string
	Stack         string
	Tags          []string
	ComponentType string
}

// PullOption is a functional option for configuring PullOptions.
type PullOption func(*PullOptions)

// WithDryRun sets the dry-run mode for vendor pull.
func WithDryRun(v bool) PullOption {
	return func(o *PullOptions) { o.DryRun = v }
}

// WithComponent sets the component to vendor.
func WithComponent(c string) PullOption {
	return func(o *PullOptions) { o.Component = c }
}

// WithStack sets the stack to vendor.
func WithStack(s string) PullOption {
	return func(o *PullOptions) { o.Stack = s }
}

// WithTags sets the tags to filter components.
func WithTags(t []string) PullOption {
	return func(o *PullOptions) { o.Tags = t }
}

// WithComponentType sets the component type (terraform, helmfile, packer).
func WithComponentType(t string) PullOption {
	return func(o *PullOptions) { o.ComponentType = t }
}

// ComponentOptions configures single component vendoring.
type ComponentOptions struct {
	DryRun        bool
	ComponentType string
}

// ComponentOption is a functional option for configuring ComponentOptions.
type ComponentOption func(*ComponentOptions)

// WithComponentDryRun sets the dry-run mode for component vendoring.
func WithComponentDryRun(v bool) ComponentOption {
	return func(o *ComponentOptions) { o.DryRun = v }
}

// WithComponentComponentType sets the component type for component vendoring.
func WithComponentComponentType(t string) ComponentOption {
	return func(o *ComponentOptions) { o.ComponentType = t }
}

// StackOptions configures stack-based vendoring.
type StackOptions struct {
	DryRun bool
}

// StackOption is a functional option for configuring StackOptions.
type StackOption func(*StackOptions)

// WithStackDryRun sets the dry-run mode for stack vendoring.
func WithStackDryRun(v bool) StackOption {
	return func(o *StackOptions) { o.DryRun = v }
}
