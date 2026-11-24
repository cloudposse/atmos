package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Every `terraform` property and every element of a body array is inspected; content that is not an HCL JSON
// body declares nothing, and its rejection is left to the caller.
func TestJSONDeclaresBackend(t *testing.T) {
	for content, want := range map[string]bool{
		`{"terraform":{"backend":{"local":{}}}}`:                                   true,
		`{"terraform":{"cloud":{"organization":"x"}}}`:                             true,
		`{"terraform":{"backend":{}},"terraform":{}}`:                              true,
		`{"locals":{"a":[1,{"b":2}]},"terraform":[{"x":1},{"backend":{"s3":{}}}]}`: true,
		`[{"output":{"a":{"value":"x"}}},{"terraform":{"cloud":{}}}]`:              true,
		`{"terraform":null,"terraform":{"backend":{"local":{}}}}`:                  true,
		`{"terraform":null}`:                     false,
		`{"terraform":{}}`:                       false,
		`{"terraform":{"required_version":"1"}}`: false,
		`[{"variable":{"a":{}}}]`:                false,
		`{"variable":{"backend":{}}}`:            false,
		`[]`:                                     false,
		`"scalar"`:                               false,
		`[1]`:                                    false,
		`{"terraform":"text"}`:                   false,
		`{"terraform":`:                          false,
		`not json`:                               false,
	} {
		assert.Equal(t, want, jsonDeclaresBackend([]byte(content)), content)
	}
}
