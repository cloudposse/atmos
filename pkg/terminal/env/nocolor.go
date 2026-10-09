package env

// ResolveNoColor reports whether color is globally disabled before Cobra has parsed flags.
//
// Precedence: a non-empty NO_COLOR is absolute; otherwise an explicit --no-color flag
// (true or false) beats the ATMOS_NO_COLOR environment variable and any viper value
// derived from the environment; with no flag, ATMOS_NO_COLOR or the supplied viper
// value (for example one set by in-process callers) enables the opt-out.
// Args excludes the executable name.
func ResolveNoColor(args []string, viperNoColor bool) bool {
	options := ColorOptionsFromArgs(args)
	if options.NoColorSet {
		return options.NoColor
	}
	return options.NoColor || viperNoColor
}
