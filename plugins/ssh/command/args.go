package command

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Flags the commands understand. Value flags take the next token or "=value".
var (
	valueFlags = []string{"root", "format", "lines", "soak", "interval", "store"}
	boolFlags  = []string{"yes", "help"}
)

// flags is a parsed command line: the words, and the flags found anywhere among them.
type flags struct {
	words  []string
	values map[string]string
	bools  map[string]bool
}

// parseFlags splits raw tokens into words and flags. "--" ends flag parsing. "-h" means "--help".
func parseFlags(args []string) (flags, error) {
	f := flags{values: map[string]string{}, bools: map[string]bool{}}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			f.words = append(f.words, args[i+1:]...)
			return f, nil
		case arg == "-h":
			f.bools["help"] = true
		case strings.HasPrefix(arg, "--") && len(arg) > 2:
			next, err := f.take(arg[2:], args, i)
			if err != nil {
				return flags{}, err
			}
			i = next
		case strings.HasPrefix(arg, "-") && len(arg) > 1:
			return flags{}, usagef("unknown flag %q", arg)
		default:
			f.words = append(f.words, arg)
		}
	}
	return f, nil
}

// take consumes the flag at args[i] and returns the index of the last token it used.
func (f *flags) take(spec string, args []string, i int) (int, error) {
	name, value, hasValue := strings.Cut(spec, "=")
	switch {
	case contains(boolFlags, name):
		if hasValue {
			return i, usagef("flag --%s takes no value", name)
		}
		f.bools[name] = true
	case contains(valueFlags, name):
		if !hasValue {
			if i+1 >= len(args) {
				return i, usagef("flag --%s needs a value", name)
			}
			i++
			value = args[i]
		}
		if value == "" {
			return i, usagef("flag --%s needs a value", name)
		}
		f.values[name] = value
	default:
		return i, usagef("unknown flag --%s", name)
	}
	return i, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// only fails when a flag other than the allowed ones was given. --root and --help are always
// allowed.
func (f flags) only(allowed ...string) error {
	for name := range f.values {
		if name != "root" && !contains(allowed, name) {
			return usagef("flag --%s does not apply to this command", name)
		}
	}
	for name := range f.bools {
		if name != "help" && !contains(allowed, name) {
			return usagef("flag --%s does not apply to this command", name)
		}
	}
	return nil
}

// exactly checks the number of positional words after the command path.
func exactly(words []string, n int, usage string) error {
	if len(words) != n {
		return usagef("usage: %s", usage)
	}
	return nil
}

func (f flags) duration(name string, def time.Duration) (time.Duration, error) {
	v, ok := f.values[name]
	if !ok {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return 0, usagef("--%s %q is not a duration such as 30s or 10m", name, v)
	}
	return d, nil
}

func (f flags) count(name string, def int) (int, error) {
	v, ok := f.values[name]
	if !ok {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 0, usagef("--%s %q is not a positive number", name, v)
	}
	return n, nil
}

// usageError is a mistake in how the command was called. It exits with code 2.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

func usagef(format string, args ...any) error {
	return &usageError{msg: fmt.Sprintf(format, args...)}
}
