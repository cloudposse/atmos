package vendoring

// DiffParams contains parameters for the Diff operation.
type DiffParams struct {
	Component     string
	ComponentType string
	From          string
	To            string
	File          string
	Context       int
	Unified       bool
	NoColor       bool
}

// UpdateParams contains parameters for the Update operation.
type UpdateParams struct {
	Component     string
	ComponentType string
	Check         bool
	Pull          bool
	Tags          string
	Outdated      bool
}
