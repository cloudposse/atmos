package exec

// Constants used for formatting and display of terraform plan diffs.
const (
	// MaxStringDisplayLength is the maximum length of a string before truncating it in output.
	maxStringDisplayLength = 100

	// HalfStringDisplayLength is half of the max string length used for truncation.
	halfStringDisplayLength = 40

	// NoChangesText is the text used to represent that no changes were found in a diff.
	noChangesText = "(no changes)"

	// DefaultValueFormat is the format string used for displaying values.
	defaultValueFormat = "%v"

	// PlanFileMode defines the file permission for plan files.
	planFileMode = 0o600
)
