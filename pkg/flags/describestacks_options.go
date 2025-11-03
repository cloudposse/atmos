package flags

// DescribeStacksOptions contains parsed flags for the describe stacks command.
type DescribeStacksOptions struct {
	GlobalFlags
	Stack              string
	Format             string
	File               string
	ProcessTemplates   bool
	ProcessFunctions   bool
	Components         []string
	ComponentTypes     []string
	Sections           []string
	IncludeEmptyStacks bool
	Skip               []string
	Query              string
}
