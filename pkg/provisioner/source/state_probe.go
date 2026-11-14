package source

import (
	"encoding/json"
	"errors"
	"io"
	"os"
)

// isTerraformState recognizes custom runtime paths without imposing a size limit.
// Inspection I/O errors must stop publication rather than transfer ownership to source.
func isTerraformState(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	state, err := inspectStateMetadata(file)
	var syntax *json.SyntaxError
	if errors.As(err, &syntax) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return false, nil // Ordinary source need not contain valid JSON.
	}
	return state, err
}

// inspectStateMetadata reads only top-level identity fields. Nested resources are
// skipped token by token, and a recognized state header ends inspection immediately.
func inspectStateMetadata(reader io.Reader) (bool, error) {
	decoder := json.NewDecoder(reader)
	decoder.UseNumber()
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return false, err
	}
	var header stateHeader
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return false, err
		}
		value, err := decoder.Token()
		if err != nil {
			return false, err
		}
		if header.observe(key, value) {
			return true, nil
		}
		if err := skipStateValue(decoder, value); err != nil {
			return false, err
		}
	}
	_, err = decoder.Token()
	return false, err
}

type stateHeader struct {
	version, lineage, serial bool
}

func (header *stateHeader) observe(key, value json.Token) bool {
	switch key {
	case "version":
		number, valid := stateInteger(value)
		header.version = valid && number > 0
	case "lineage":
		text, ok := value.(string)
		header.lineage = ok && text != ""
	case "serial":
		_, header.serial = stateInteger(value)
	}
	return header.version && header.lineage && header.serial
}

func stateInteger(token json.Token) (int64, bool) {
	number, ok := token.(json.Number)
	if !ok {
		return 0, false
	}
	value, err := number.Int64()
	return value, err == nil
}

func skipStateValue(decoder *json.Decoder, first json.Token) error {
	if first != json.Delim('{') && first != json.Delim('[') {
		return nil
	}
	for depth := 1; depth > 0; {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		switch token {
		case json.Delim('{'), json.Delim('['):
			depth++
		case json.Delim('}'), json.Delim(']'):
			depth--
		}
	}
	return nil
}
