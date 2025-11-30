package provenance

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestArrayOfMapsPathBuilding tests that array items with keys (- name: value)
// get properly indexed paths like vars.items[0].name instead of vars.items.name.
func TestArrayOfMapsPathBuilding(t *testing.T) {
	yaml := `vars:
  items:
    - name: widget
      color: blue
    - name: gadget
      color: red`

	lines := strings.Split(yaml, "\n")
	pathMap := buildYAMLPathMap(lines)

	// Line 2: "- name: widget" should be vars.items[0].name
	if info, ok := pathMap[2]; ok {
		assert.Equal(t, "vars.items[0].name", info.Path, "First array element 'name' should have indexed path")
		assert.True(t, info.IsKeyLine, "Should be marked as a key line")
	} else {
		t.Fatal("Line 2 not found in path map")
	}

	// Line 3: "color: blue" should be vars.items[0].color
	if info, ok := pathMap[3]; ok {
		assert.Equal(t, "vars.items[0].color", info.Path, "First array element 'color' should have indexed path")
		assert.True(t, info.IsKeyLine, "Should be marked as a key line")
	} else {
		t.Fatal("Line 3 not found in path map")
	}

	// Line 4: "- name: gadget" should be vars.items[1].name
	if info, ok := pathMap[4]; ok {
		assert.Equal(t, "vars.items[1].name", info.Path, "Second array element 'name' should have indexed path")
		assert.True(t, info.IsKeyLine, "Should be marked as a key line")
	} else {
		t.Fatal("Line 4 not found in path map")
	}

	// Line 5: "color: red" should be vars.items[1].color
	if info, ok := pathMap[5]; ok {
		assert.Equal(t, "vars.items[1].color", info.Path, "Second array element 'color' should have indexed path")
		assert.True(t, info.IsKeyLine, "Should be marked as a key line")
	} else {
		t.Fatal("Line 5 not found in path map")
	}
}

// TestNestedArrayOfMaps tests nested structures with arrays of maps.
func TestNestedArrayOfMaps(t *testing.T) {
	yaml := `config:
  servers:
    - name: web01
      ports:
        - port: 80
          protocol: http
        - port: 443
          protocol: https`

	lines := strings.Split(yaml, "\n")
	pathMap := buildYAMLPathMap(lines)

	// Line 2: "- name: web01" should be config.servers[0].name
	if info, ok := pathMap[2]; ok {
		assert.Equal(t, "config.servers[0].name", info.Path, "Server name should have indexed path")
	} else {
		t.Fatal("Line 2 not found in path map")
	}

	// Line 4: "- port: 80" should be config.servers[0].ports[0].port
	if info, ok := pathMap[4]; ok {
		assert.Equal(t, "config.servers[0].ports[0].port", info.Path, "Nested array port should have double-indexed path")
	} else {
		t.Fatal("Line 4 not found in path map")
	}

	// Line 5: "protocol: http" should be config.servers[0].ports[0].protocol
	if info, ok := pathMap[5]; ok {
		assert.Equal(t, "config.servers[0].ports[0].protocol", info.Path, "Nested array protocol should have double-indexed path")
	} else {
		t.Fatal("Line 5 not found in path map")
	}

	// Line 6: "- port: 443" should be config.servers[0].ports[1].port
	if info, ok := pathMap[6]; ok {
		assert.Equal(t, "config.servers[0].ports[1].port", info.Path, "Second port entry should have index [1]")
	} else {
		t.Fatal("Line 6 not found in path map")
	}
}

// TestArrayOfMapsWithNestedObjects tests arrays of maps containing nested objects.
func TestArrayOfMapsWithNestedObjects(t *testing.T) {
	yaml := `components:
  terraform:
    vpc:
      vars:
        subnets:
          - cidr: 10.0.1.0/24
            az: us-east-1a
            tags:
              Name: public-1
              Tier: public`

	lines := strings.Split(yaml, "\n")
	pathMap := buildYAMLPathMap(lines)

	// Line 5: "- cidr: 10.0.1.0/24" should be components.terraform.vpc.vars.subnets[0].cidr
	if info, ok := pathMap[5]; ok {
		assert.Equal(t, "components.terraform.vpc.vars.subnets[0].cidr", info.Path,
			"Array element with nested object should have indexed path")
	} else {
		t.Fatal("Line 5 not found in path map")
	}

	// Line 8: "Name: public-1" should be components.terraform.vpc.vars.subnets[0].tags.Name
	if info, ok := pathMap[8]; ok {
		assert.Equal(t, "components.terraform.vpc.vars.subnets[0].tags.Name", info.Path,
			"Nested object field should maintain array index")
	} else {
		t.Fatal("Line 8 not found in path map")
	}
}

func TestSimpleArrayDebug(t *testing.T) {
	yaml := `vars:
  items:
    - name: widget
      color: blue`

	lines := strings.Split(yaml, "\n")
	pathMap := buildYAMLPathMap(lines)

	t.Cleanup(func() {
		if t.Failed() {
			t.Log("\n=== PATH MAP ===")
			for i := 0; i < len(lines); i++ {
				if info, ok := pathMap[i]; ok {
					t.Logf("Line %d: %q -> Path: %q", i, strings.TrimSpace(lines[i]), info.Path)
				}
			}
		}
	})
}

// TestRootLevelArrayOfScalars tests that root-level arrays of scalars get indexed correctly.
//
//nolint:dupl // Test structure mirrors TestNestedArrayOfScalars for consistency.
func TestRootLevelArrayOfScalars(t *testing.T) {
	yaml := `- item1
- item2
- item3`

	lines := strings.Split(yaml, "\n")
	pathMap := buildYAMLPathMap(lines)

	// Debug: print all paths on failure
	t.Cleanup(func() {
		if t.Failed() {
			t.Log("\n=== ROOT SCALAR ARRAY PATH MAP ===")
			for i := 0; i < len(lines); i++ {
				if info, ok := pathMap[i]; ok {
					t.Logf("Line %d: %q -> Path: %q", i, strings.TrimSpace(lines[i]), info.Path)
				}
			}
		}
	})

	// Line 0: "- item1" should be [0]
	if info, ok := pathMap[0]; ok {
		assert.Equal(t, "[0]", info.Path, "First root scalar array element should have indexed path")
		assert.True(t, info.IsKeyLine, "Should be marked as a key line")
	} else {
		t.Fatal("Line 0 not found in path map")
	}

	// Line 1: "- item2" should be [1]
	if info, ok := pathMap[1]; ok {
		assert.Equal(t, "[1]", info.Path, "Second root scalar array element should have indexed path")
		assert.True(t, info.IsKeyLine, "Should be marked as a key line")
	} else {
		t.Fatal("Line 1 not found in path map")
	}

	// Line 2: "- item3" should be [2]
	if info, ok := pathMap[2]; ok {
		assert.Equal(t, "[2]", info.Path, "Third root scalar array element should have indexed path")
		assert.True(t, info.IsKeyLine, "Should be marked as a key line")
	} else {
		t.Fatal("Line 2 not found in path map")
	}
}

// TestRootLevelArrayOfMaps tests that root-level array-of-maps get indexed correctly.
func TestRootLevelArrayOfMaps(t *testing.T) {
	yaml := `- key: value1
  name: first
- key: value2
  name: second`

	lines := strings.Split(yaml, "\n")
	pathMap := buildYAMLPathMap(lines)

	// Debug: print all paths on failure
	t.Cleanup(func() {
		if t.Failed() {
			t.Log("\n=== ROOT ARRAY PATH MAP ===")
			for i := 0; i < len(lines); i++ {
				if info, ok := pathMap[i]; ok {
					t.Logf("Line %d: %q -> Path: %q", i, strings.TrimSpace(lines[i]), info.Path)
				}
			}
		}
	})

	// Line 0: "- key: value1" should be [0].key
	if info, ok := pathMap[0]; ok {
		assert.Equal(t, "[0].key", info.Path, "First root array element 'key' should have indexed path")
		assert.True(t, info.IsKeyLine, "Should be marked as a key line")
	} else {
		t.Fatal("Line 0 not found in path map")
	}

	// Line 1: "  name: first" should be [0].name
	if info, ok := pathMap[1]; ok {
		assert.Equal(t, "[0].name", info.Path, "First root array element 'name' should have indexed path")
		assert.True(t, info.IsKeyLine, "Should be marked as a key line")
	} else {
		t.Fatal("Line 1 not found in path map")
	}

	// Line 2: "- key: value2" should be [1].key
	if info, ok := pathMap[2]; ok {
		assert.Equal(t, "[1].key", info.Path, "Second root array element 'key' should have indexed path")
		assert.True(t, info.IsKeyLine, "Should be marked as a key line")
	} else {
		t.Fatal("Line 2 not found in path map")
	}

	// Line 3: "  name: second" should be [1].name
	if info, ok := pathMap[3]; ok {
		assert.Equal(t, "[1].name", info.Path, "Second root array element 'name' should have indexed path")
		assert.True(t, info.IsKeyLine, "Should be marked as a key line")
	} else {
		t.Fatal("Line 3 not found in path map")
	}
}

// TestNestedArrayOfScalars tests that scalar array items get full path prefixes.
// This is a regression test for the bug where handleArrayItemLine only used the
// last stack element, producing "items[0]" instead of "vars.items[0]".
//
//nolint:dupl // Test structure mirrors TestRootLevelArrayOfScalars for consistency.
func TestNestedArrayOfScalars(t *testing.T) {
	yaml := `vars:
  items:
    - scalar1
    - scalar2
    - scalar3`

	lines := strings.Split(yaml, "\n")
	pathMap := buildYAMLPathMap(lines)

	// Debug: print all paths on failure
	t.Cleanup(func() {
		if t.Failed() {
			t.Log("\n=== NESTED SCALAR ARRAY PATH MAP ===")
			for i := 0; i < len(lines); i++ {
				if info, ok := pathMap[i]; ok {
					t.Logf("Line %d: %q -> Path: %q", i, strings.TrimSpace(lines[i]), info.Path)
				}
			}
		}
	})

	// Line 2: "- scalar1" should be vars.items[0] (not just items[0])
	if info, ok := pathMap[2]; ok {
		assert.Equal(t, "vars.items[0]", info.Path, "First scalar should have full path prefix")
		assert.True(t, info.IsKeyLine, "Should be marked as a key line")
	} else {
		t.Fatal("Line 2 not found in path map")
	}

	// Line 3: "- scalar2" should be vars.items[1]
	if info, ok := pathMap[3]; ok {
		assert.Equal(t, "vars.items[1]", info.Path, "Second scalar should have full path prefix")
		assert.True(t, info.IsKeyLine, "Should be marked as a key line")
	} else {
		t.Fatal("Line 3 not found in path map")
	}

	// Line 4: "- scalar3" should be vars.items[2]
	if info, ok := pathMap[4]; ok {
		assert.Equal(t, "vars.items[2]", info.Path, "Third scalar should have full path prefix")
		assert.True(t, info.IsKeyLine, "Should be marked as a key line")
	} else {
		t.Fatal("Line 4 not found in path map")
	}
}

// TestRootArrayWithNestedMaps tests root-level array elements that contain nested maps.
// This is a regression test for the bug where arrayIndexStack wasn't popped correctly
// when exiting nested scopes at root level, causing all siblings to be tagged as [0].
func TestRootArrayWithNestedMaps(t *testing.T) {
	yaml := `- metadata:
    name: first
    id: 1
- metadata:
    name: second
    id: 2
- metadata:
    name: third
    id: 3`

	lines := strings.Split(yaml, "\n")
	pathMap := buildYAMLPathMap(lines)

	// Debug: print all paths on failure
	t.Cleanup(func() {
		if t.Failed() {
			t.Log("\n=== ROOT ARRAY WITH NESTED MAPS PATH MAP ===")
			for i := 0; i < len(lines); i++ {
				if info, ok := pathMap[i]; ok {
					t.Logf("Line %d: %q -> Path: %q", i, strings.TrimSpace(lines[i]), info.Path)
				}
			}
		}
	})

	// Line 0: "- metadata:" should be [0].metadata
	if info, ok := pathMap[0]; ok {
		assert.Equal(t, "[0].metadata", info.Path, "First element metadata should be [0].metadata")
		assert.True(t, info.IsKeyLine, "Should be marked as a key line")
	} else {
		t.Fatal("Line 0 not found in path map")
	}

	// Line 1: "    name: first" should be [0].metadata.name
	if info, ok := pathMap[1]; ok {
		assert.Equal(t, "[0].metadata.name", info.Path, "First element name should be [0].metadata.name")
		assert.True(t, info.IsKeyLine, "Should be marked as a key line")
	} else {
		t.Fatal("Line 1 not found in path map")
	}

	// Line 3: "- metadata:" should be [1].metadata (NOT [0].metadata)
	if info, ok := pathMap[3]; ok {
		assert.Equal(t, "[1].metadata", info.Path, "Second element metadata should be [1].metadata")
		assert.True(t, info.IsKeyLine, "Should be marked as a key line")
	} else {
		t.Fatal("Line 3 not found in path map")
	}

	// Line 4: "    name: second" should be [1].metadata.name
	if info, ok := pathMap[4]; ok {
		assert.Equal(t, "[1].metadata.name", info.Path, "Second element name should be [1].metadata.name")
		assert.True(t, info.IsKeyLine, "Should be marked as a key line")
	} else {
		t.Fatal("Line 4 not found in path map")
	}

	// Line 6: "- metadata:" should be [2].metadata (NOT [0].metadata)
	if info, ok := pathMap[6]; ok {
		assert.Equal(t, "[2].metadata", info.Path, "Third element metadata should be [2].metadata")
		assert.True(t, info.IsKeyLine, "Should be marked as a key line")
	} else {
		t.Fatal("Line 6 not found in path map")
	}

	// Line 7: "    name: third" should be [2].metadata.name
	if info, ok := pathMap[7]; ok {
		assert.Equal(t, "[2].metadata.name", info.Path, "Third element name should be [2].metadata.name")
		assert.True(t, info.IsKeyLine, "Should be marked as a key line")
	} else {
		t.Fatal("Line 7 not found in path map")
	}
}
