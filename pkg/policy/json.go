package policy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
)

const (
	jsonNumberBits = 64
	decimalBase    = 10
)

// decodeObject preserves integer IDs instead of silently rounding through float64.
// It accepts only materialized JSON, never arbitrary Go values with lazy methods.
func decodeObject(data json.RawMessage) (map[string]any, error) {
	if !json.Valid(data) {
		return nil, fmt.Errorf("%w: expected valid JSON object", errUtils.ErrPolicyInputInvalid)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrPolicyInputInvalid, err)
	}
	if object == nil {
		return nil, fmt.Errorf("%w: expected object, got null", errUtils.ErrPolicyInputInvalid)
	}
	_, err := normalizeNumbers(object)
	return object, err
}

func normalizeNumbers(value any) (any, error) {
	switch item := value.(type) {
	case json.Number:
		return decodeNumber(item)
	case map[string]any:
		for key, entry := range item {
			converted, err := normalizeNumbers(entry)
			if err != nil {
				return nil, err
			}
			item[key] = converted
		}
	case []any:
		for index, entry := range item {
			converted, err := normalizeNumbers(entry)
			if err != nil {
				return nil, err
			}
			item[index] = converted
		}
	}
	return value, nil
}

func decodeNumber(number json.Number) (any, error) {
	text := number.String()
	if strings.ContainsAny(text, ".eE") {
		value, err := strconv.ParseFloat(text, jsonNumberBits)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errUtils.ErrPolicyInputInvalid, err)
		}
		return value, nil
	}
	if value, err := number.Int64(); err == nil {
		return value, nil
	}
	value, err := strconv.ParseUint(text, decimalBase, jsonNumberBits)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrPolicyInputInvalid, err)
	}
	return value, nil
}
