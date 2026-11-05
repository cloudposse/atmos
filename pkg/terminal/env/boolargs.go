package env

import "strings"

// IsBoolLiteral reports whether value is exactly "true" or "false", ignoring case.
// Other spellings accepted by strconv.ParseBool, such as 1, 0, t, or f, are deliberately
// excluded because they are far more likely to be a positional argument than a
// boolean flag value.
// This package deliberately has no Atmos dependencies so the logger can use it
// during package initialization.
func IsBoolLiteral(value string) bool {
	return strings.EqualFold(value, "true") || strings.EqualFold(value, "false")
}

// NormalizeBoolFlagValues rewrites "--flag true|false" to "--flag=true|false" for
// flags that isBoolFlag reports as boolean.
//
// Cobra/pflag boolean flags never consume the following argument, so
// "--no-color false" leaves "false" behind as a stray positional argument (an
// unknown command, or an argument forwarded to Terraform). Only the exact
// literals "true" and "false" (case-insensitive) are folded into the flag; any
// other following argument (a subcommand, a component name, another flag) is
// left alone so that a bare "--flag" keeps meaning "true".
//
// The isBoolFlag callback receives the flag name without leading dashes: the long
// name for "--name" and the shorthand for "-n". Arguments that already carry a
// value ("--flag=false"), unknown flags, and everything after the "--" terminator
// are never modified. The input slice is not mutated; it is returned as-is when
// nothing needs rewriting.
func NormalizeBoolFlagValues(args []string, isBoolFlag func(name string) bool) []string {
	if isBoolFlag == nil {
		return args
	}

	// Flags after the "--" terminator belong to another tool and are never rewritten.
	scanEnd := len(args)
	for i, arg := range args {
		if arg == "--" {
			scanEnd = i
			break
		}
	}

	head := args[:scanEnd]
	var result []string
	for i := 0; i < len(head); i++ {
		arg := head[i]
		if i+1 < len(head) && shouldFoldBoolValue(arg, head[i+1], isBoolFlag) {
			if result == nil {
				result = make([]string, i, len(args))
				copy(result, head[:i])
			}
			result = append(result, arg+"="+head[i+1])
			i++ // The value was folded into the flag.
			continue
		}
		if result != nil {
			result = append(result, arg)
		}
	}
	if result == nil {
		return args
	}
	return append(result, args[scanEnd:]...)
}

// shouldFoldBoolValue reports whether arg is a registered boolean flag in bare
// form and next is an explicit true/false literal.
func shouldFoldBoolValue(arg, next string, isBoolFlag func(name string) bool) bool {
	if !IsBoolLiteral(next) || strings.Contains(arg, "=") {
		return false
	}
	switch {
	case strings.HasPrefix(arg, "--"):
		name := strings.TrimPrefix(arg, "--")
		return len(name) > 1 && isBoolFlag(name)
	case strings.HasPrefix(arg, "-"):
		name := strings.TrimPrefix(arg, "-")
		return len(name) == 1 && isBoolFlag(name)
	default:
		return false
	}
}
