package main

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	formatDecimalBase          = 10
	maxQuotedBytesPerInputByte = 6
	maxHexBytesPerInputByte    = 5
)

// environmentFormat checks each directive before fmt can allocate its result.
// Arguments are data scalars, never values with user-defined formatting methods.
type environmentFormat struct {
	source string
	args   []any
	pos    int
	next   int
}

type environmentDirective struct {
	flags     string
	width     int
	precision int
	verb      rune
	argument  any
}

func boundedPrintf(source string, args ...any) (string, error) {
	if len(source) > maxEnvironmentValueBytes {
		return "", errEnvironmentTemplate
	}
	parser := environmentFormat{source: source, args: args}
	var output boundedEnvironmentValue
	for parser.pos < len(source) {
		literal := strings.IndexByte(source[parser.pos:], '%')
		if literal < 0 {
			literal = len(source) - parser.pos
		}
		if _, err := output.Write([]byte(source[parser.pos : parser.pos+literal])); err != nil {
			return "", err
		}
		parser.pos += literal
		if parser.pos == len(source) {
			break
		}
		directive, err := parser.directive()
		if err != nil {
			return "", err
		}
		if err := directive.write(&output); err != nil {
			return "", err
		}
	}
	return output.buffer.String(), nil
}

func (parser *environmentFormat) directive() (environmentDirective, error) {
	parser.pos++
	directive := environmentDirective{width: -1, precision: -1}
	start := parser.pos
	for parser.pos < len(parser.source) && strings.ContainsRune("+#- 0", rune(parser.source[parser.pos])) {
		parser.pos++
	}
	directive.flags = parser.source[start:parser.pos]
	if err := parser.argumentIndex(); err != nil {
		return directive, err
	}
	var err error
	dynamicWidth := parser.pos < len(parser.source) && parser.source[parser.pos] == '*'
	directive.width, err = parser.dimension()
	if err != nil {
		return directive, err
	}
	if dynamicWidth && directive.width < 0 {
		directive.width = -directive.width
		directive.flags += "-"
	}
	directive.precision, err = parser.precision()
	if err != nil {
		return directive, err
	}
	return parser.directiveArgument(directive)
}

func (parser *environmentFormat) precision() (int, error) {
	if parser.pos >= len(parser.source) || parser.source[parser.pos] != '.' {
		return -1, nil
	}
	parser.pos++
	if err := parser.argumentIndex(); err != nil {
		return 0, err
	}
	dynamic := parser.pos < len(parser.source) && parser.source[parser.pos] == '*'
	precision, err := parser.dimension()
	if err != nil {
		return 0, err
	}
	// A bare dot means zero; negative dynamic precision is invalid.
	if precision == -1 && !dynamic {
		return 0, nil
	}
	if precision < 0 {
		return 0, errEnvironmentTemplate
	}
	return precision, nil
}

func (parser *environmentFormat) directiveArgument(directive environmentDirective) (environmentDirective, error) {
	if err := parser.argumentIndex(); err != nil || parser.pos >= len(parser.source) {
		return directive, errEnvironmentTemplate
	}
	verb, size := utf8.DecodeRuneInString(parser.source[parser.pos:])
	parser.pos += size
	directive.verb = verb
	if verb != '%' {
		if parser.next >= len(parser.args) {
			return directive, errEnvironmentTemplate
		}
		directive.argument = parser.args[parser.next]
		parser.next++
	}
	return directive, nil
}

func (parser *environmentFormat) argumentIndex() error {
	if parser.pos >= len(parser.source) || parser.source[parser.pos] != '[' {
		return nil
	}
	parser.pos++
	index, err := parser.number()
	if err != nil || index < 1 || index > len(parser.args) || parser.pos >= len(parser.source) || parser.source[parser.pos] != ']' {
		return errEnvironmentTemplate
	}
	parser.pos++
	parser.next = index - 1
	return nil
}

func (parser *environmentFormat) number() (int, error) {
	number := -1
	for parser.pos < len(parser.source) && parser.source[parser.pos] >= '0' && parser.source[parser.pos] <= '9' {
		if number < 0 {
			number = 0
		}
		number = number*formatDecimalBase + int(parser.source[parser.pos]-'0')
		parser.pos++
		if number > maxEnvironmentValueBytes {
			return 0, errEnvironmentTemplate
		}
	}
	return number, nil
}

func (parser *environmentFormat) dimension() (int, error) {
	if parser.pos >= len(parser.source) || parser.source[parser.pos] != '*' {
		return parser.number()
	}
	parser.pos++
	if parser.next >= len(parser.args) {
		return 0, errEnvironmentTemplate
	}
	value := reflect.ValueOf(parser.args[parser.next])
	parser.next++
	return environmentDimension(value)
}

func environmentDimension(value reflect.Value) (int, error) {
	if !value.IsValid() {
		return 0, errEnvironmentTemplate
	}
	if value.CanUint() {
		number := value.Uint()
		if number > maxEnvironmentValueBytes {
			return 0, errEnvironmentTemplate
		}
		return int(number), nil
	}
	if !value.CanInt() || value.Int() > maxEnvironmentValueBytes || value.Int() < -maxEnvironmentValueBytes {
		return 0, errEnvironmentTemplate
	}
	return int(value.Int()), nil
}

func (directive environmentDirective) write(output *boundedEnvironmentValue) error {
	if directive.verb == '%' {
		_, err := output.Write([]byte{'%'})
		return err
	}
	bound, err := directive.bound()
	if err != nil || bound > maxEnvironmentValueBytes-output.buffer.Len() {
		return errEnvironmentTemplate
	}
	format := "%" + directive.flags
	if directive.width >= 0 {
		format += strconv.Itoa(directive.width)
	}
	if directive.precision >= 0 {
		format += "." + strconv.Itoa(directive.precision)
	}
	format += string(directive.verb)
	_, err = fmt.Fprintf(output, format, directive.argument)
	return err
}

func (directive environmentDirective) bound() (int, error) {
	if text, ok := directive.argument.(string); ok {
		return directive.stringBound(text)
	}
	// These are the only scalar types Go templates and credential data expose.
	// Reject composites and custom formatters before invoking fmt.
	if !strings.ContainsRune("vTtbcdoOxXUqefFgG", directive.verb) {
		return 0, errEnvironmentTemplate
	}
	switch directive.argument.(type) {
	case complex64, complex128:
		return 2*max(directive.width, max(directive.precision, 0)+400) + 3, nil
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, uintptr, float32, float64:
		return max(directive.width, max(directive.precision, 0)+400), nil
	default:
		return 0, errEnvironmentTemplate
	}
}

func (directive environmentDirective) stringBound(text string) (int, error) {
	if directive.verb == 'T' {
		return max(directive.width, len("string")), nil
	}
	if directive.precision >= 0 {
		text = truncateFormatString(text, directive.precision, directive.verb)
	}
	length := len(text)
	switch directive.verb {
	case 's', 'v':
		if directive.verb == 'v' && strings.Contains(directive.flags, "#") {
			length = maxQuotedBytesPerInputByte*length + 2
		}
	case 'q':
		length = maxQuotedBytesPerInputByte*length + 2
	case 'x', 'X':
		length = maxHexBytesPerInputByte * length
	default:
		return 0, errEnvironmentTemplate
	}
	return length + max(0, directive.width-utf8.RuneCountInString(text)), nil
}

func truncateFormatString(text string, precision int, verb rune) string {
	if verb == 'x' || verb == 'X' {
		return text[:min(len(text), precision)]
	}
	for index := range text {
		if precision == 0 {
			return text[:index]
		}
		precision--
	}
	return text
}
