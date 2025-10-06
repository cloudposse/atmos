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

	t.Log("\n=== PATH MAP ===")
	for i := 0; i < len(lines); i++ {
		if info, ok := pathMap[i]; ok {
			t.Logf("Line %d: %q -> Path: %q", i, strings.TrimSpace(lines[i]), info.Path)
		}
	}
}
