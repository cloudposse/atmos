# Test Fixture: `!template` YAML Function

## Purpose

This test fixture validates the `!template` YAML function implementation in Atmos.

## Background

The `!template` function solves a specific problem with Go template expressions in YAML:

**Problem**: When you use `toJson` in a Go template expression (e.g., to get complex outputs from `atmos.Component()`),
the result is a JSON-encoded string. In YAML, this remains a string literal rather than being parsed as a native YAML
structure.

**Solution**: The `!template` tag tells Atmos to decode the JSON string back into native YAML types (maps, lists, etc.).

## Test Coverage

This fixture tests the following scenarios:

### Basic Functionality (Component: `test-basic-template`)

- Simple strings (no JSON encoding)
- JSON-encoded primitives (string, number, boolean, null)
- JSON-encoded lists
- JSON-encoded maps
- Complex nested JSON structures
- Invalid JSON (graceful degradation)
- Empty strings (edge case)

### Go Template Expressions (Component: `test-template-with-expressions`)

- Template expressions resolving to strings
- Template expressions with `toJson` for lists
- Template expressions with `toJson` for maps
- Template expressions accessing stack variables
- Nested template expressions with complex structures

### Integration with `atmos.Component()` (Component: `test-template-with-atmos-component`)

- Getting string outputs (no `!template` needed)
- Getting list outputs with `toJson` + `!template`
- Getting map outputs with `toJson` + `!template`
- Getting nested objects with `toJson` + `!template`
- Extracting nested paths from component outputs
- Extracting specific values from nested objects

**This is the primary documented use case:**

```yaml
# WITHOUT !template - Results in JSON string
var1: '{{ toJson (atmos.Component "vpc" .stack).outputs.subnet_ids }}'
# Result: "[\"subnet-abc\", \"subnet-def\"]"  (string)

# WITH !template - Results in native YAML list
var2: !template '{{ toJson (atmos.Component "vpc" .stack).outputs.subnet_ids }}'
# Result:
#   - subnet-abc
#   - subnet-def
```

### Lists and Maps (Components: `test-template-in-lists`, `test-template-in-maps`)

- Multiple `!template` calls in a list
- Mixed lists (static values + `!template` results)
- Multiple `!template` values in a map
- Nested structures with `!template` at various levels

### Edge Cases (Component: `test-template-edge-cases`)

- Whitespace handling (before, after, both)
- Escaped quotes in JSON
- Unicode characters (emoji, non-ASCII)
- Deep nested structures
- Arrays of objects
- Empty JSON structures ([], {})
- Mixed type arrays

### Real-World Scenario (Component: `test-real-world-scenario`)

Simulates practical use case: Getting outputs from deployed components and using them in other components

- VPC subnet IDs as native list
- Security group rules as native map
- Complex configuration objects
- Combining static config with dynamic outputs

## Implementation Details

**File**: `internal/exec/yaml_func_template.go:12-30`

**Processing Flow**:

1. Extract string after `!template` tag
2. Attempt to unmarshal as JSON using `json.Unmarshal()`
3. If successful, return decoded Go structure
4. If JSON decode fails, return original string (graceful degradation)

**Key Distinction**: `!template` is processed during stack manifest processing (not atmos.yaml preprocessing), making it
available for use in stack configurations.

## Usage

This fixture can be used to test:

1. Basic `!template` functionality with `atmos describe component` commands
2. Integration with template processing pipeline
3. Error handling and edge cases
4. Real-world usage patterns

## Example Commands

```bash
# Test basic template functionality
atmos describe component test-basic-template -s nonprod

# Test template with expressions
atmos describe component test-template-with-expressions -s nonprod

# Test template with atmos.Component() integration
atmos describe component test-template-with-atmos-component -s nonprod

# Test edge cases
atmos describe component test-template-edge-cases -s nonprod
```

## Expected Behavior

All JSON strings should be properly decoded into native YAML structures when using `!template`, while invalid JSON
should gracefully return the original string without errors.
