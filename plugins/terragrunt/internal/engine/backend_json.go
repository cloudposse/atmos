package engine

import (
	"bytes"
	"encoding/json"
	"errors"
)

// errNotJSONBody marks JSON that is not an HCL JSON body: an object, or an array of objects.
var errNotJSONBody = errors.New("not an HCL JSON body")

// jsonDeclaresBackend reports whether HCL JSON content declares a `backend` or `cloud` block. HCL JSON keeps
// duplicate properties and allows a body to be an array of objects, so the content is read token by token:
// every `terraform` property is inspected in document order, and a body array is inspected element by
// element. Content that is not valid JSON declares nothing here; callers that must prove the absence of a
// backend reject it separately.
func jsonDeclaresBackend(content []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(content))
	found, err := jsonBodyDeclaresBackend(decoder, func(key string) (bool, error) {
		if key == "terraform" {
			return jsonBodyDeclaresBackend(decoder, func(nested string) (bool, error) {
				return nested == "backend" || nested == "cloud", skipJSONValue(decoder)
			})
		}
		return false, skipJSONValue(decoder)
	})
	return err == nil && found
}

// jsonBodyDeclaresBackend reads one body, an object or an array of objects, and calls property for the key of
// each property, with the decoder positioned at its value. It stops at the first property that reports true.
func jsonBodyDeclaresBackend(decoder *json.Decoder, property func(string) (bool, error)) (bool, error) {
	token, err := decoder.Token()
	if err != nil {
		return false, err
	}
	switch token {
	case nil:
		// A null body (`"terraform": null`) declares nothing; later duplicate properties are still read.
		return false, nil
	case json.Delim('{'):
		return jsonObjectDeclaresBackend(decoder, property)
	case json.Delim('['):
		for decoder.More() {
			if opening, err := decoder.Token(); err != nil || opening != json.Delim('{') {
				return false, errors.Join(errNotJSONBody, err)
			}
			if found, err := jsonObjectDeclaresBackend(decoder, property); found || err != nil {
				return found, err
			}
		}
		_, err = decoder.Token()
		return false, err
	}
	return false, errNotJSONBody
}

// jsonObjectDeclaresBackend reads the properties of an object whose opening brace was already read.
func jsonObjectDeclaresBackend(decoder *json.Decoder, property func(string) (bool, error)) (bool, error) {
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return false, err
		}
		// The decoder only returns a string token in a key position.
		key, _ := token.(string)
		if found, err := property(key); found || err != nil {
			return found, err
		}
	}
	_, err := decoder.Token()
	return false, err
}

// skipJSONValue reads one complete value, of any depth.
func skipJSONValue(decoder *json.Decoder) error {
	depth := 0
	for {
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
		if depth == 0 {
			return nil
		}
	}
}
