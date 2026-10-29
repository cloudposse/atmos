package main

import (
	"io"
	"text/template"
)

func environmentFunctions() template.FuncMap {
	return template.FuncMap{
		"index": strictIndex, "printf": boundedPrintf, "_environmentOutput": environmentOutput,
		"print":    func(args ...any) (string, error) { return boundedPrint(false, args...) },
		"println":  func(args ...any) (string, error) { return boundedPrint(true, args...) },
		"html":     func(args ...any) (string, error) { return boundedEscape(template.HTMLEscape, args...) },
		"js":       func(args ...any) (string, error) { return boundedEscape(template.JSEscape, args...) },
		"urlquery": boundedURLQuery,
	}
}

func boundedPrint(newline bool, args ...any) (string, error) {
	var output boundedEnvironmentValue
	previousString := false
	for index, argument := range args {
		_, isString := argument.(string)
		if index > 0 && (newline || (!previousString && !isString)) {
			if _, err := output.Write([]byte{' '}); err != nil {
				return "", err
			}
		}
		directive := environmentDirective{verb: 'v', argument: argument, width: -1, precision: -1}
		if err := directive.write(&output); err != nil {
			return "", err
		}
		previousString = isString
	}
	if newline {
		if _, err := output.Write([]byte{'\n'}); err != nil {
			return "", err
		}
	}
	return output.buffer.String(), nil
}

type environmentEscapeWriter struct {
	boundedEnvironmentValue
	err error
}

func (output *environmentEscapeWriter) Write(data []byte) (int, error) {
	if output.err != nil {
		return 0, output.err
	}
	count, err := output.boundedEnvironmentValue.Write(data)
	output.err = err
	return count, err
}

func boundedEscape(escape func(io.Writer, []byte), args ...any) (string, error) {
	text, err := boundedPrint(false, args...)
	if err != nil {
		return "", err
	}
	var output environmentEscapeWriter
	escape(&output, []byte(text))
	if output.err != nil {
		return "", output.err
	}
	return output.buffer.String(), nil
}

func boundedURLQuery(args ...any) (string, error) {
	text, err := boundedPrint(false, args...)
	if err != nil {
		return "", err
	}
	var output boundedEnvironmentValue
	const hex = "0123456789ABCDEF"
	const hexNibbleMask = 15
	for index := range len(text) {
		value := text[index]
		encoded := []byte{value}
		switch {
		case value == ' ':
			encoded[0] = '+'
		case value >= 'a' && value <= 'z', value >= 'A' && value <= 'Z', value >= '0' && value <= '9', value == '-', value == '_', value == '.', value == '~':
		default:
			encoded = []byte{'%', hex[value>>4], hex[value&hexNibbleMask]}
		}
		if _, err := output.Write(encoded); err != nil {
			return "", err
		}
	}
	return output.buffer.String(), nil
}
