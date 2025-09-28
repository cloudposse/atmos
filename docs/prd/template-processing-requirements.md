# Template Processing Requirements

## Overview
This document defines the requirements for when and how Atmos processes files as Go templates during stack imports.

## Core Requirements

### R1: Template Processing Decision Logic
Files should be processed as Go templates based on the following rules:

1. **ALWAYS process as template if:**
   - File has `.yaml.tmpl` extension, OR
   - File has `.yml.tmpl` extension, OR
   - File has `.tmpl` extension, OR
   - Context is provided during import (regardless of file extension)

2. **NEVER process as template if:**
   - File has no template extension AND
   - No context is provided during import

### R2: No Template Syntax Leakage
- Template syntax (`{{ }}`) MUST NEVER appear in processed output when templates are expected to be processed
- This is a critical requirement - any leakage indicates a processing failure

### R3: Backward Compatibility
- Plain YAML files with context MUST be processed as templates (historical behavior)
- This ensures existing configurations continue to work

## Examples

### Example 1: Plain YAML with Context
```yaml
# Import statement
import:
  - path: config.yaml
    context:
      environment: "prod"

# config.yaml content
vars:
  env: "{{ .environment }}"

# Expected: Templates processed, env = "prod"
```

### Example 2: Template File without Context
```yaml
# Import statement
import:
  - path: config.yaml.tmpl

# config.yaml.tmpl content
vars:
  env: "{{ .environment | default \"dev\" }}"

# Expected: Template processed (may use defaults)
```

### Example 3: Plain YAML without Context
```yaml
# Import statement
import:
  - path: config.yaml

# config.yaml content
vars:
  literal: "This {{ stays }} as is"

# Expected: No processing, literals preserved
```

## Testing Requirements
- Unit tests for decision logic function
- Integration tests for each scenario
- Regression tests to detect template syntax leakage

## Implementation Notes

### Decision Function
The template processing decision is implemented in the `ShouldProcessFileAsTemplate` function, which:
- Takes file path, context, and skip flag as inputs
- Returns boolean indicating whether to process as template
- Is fully unit tested to verify all scenarios

### Template Processing Flow
1. Import directive is parsed
2. Decision function determines if template processing needed
3. If yes, file content is processed through Go template engine with provided context
4. If no, file content is used as-is
5. Result is merged into stack configuration

## Rationale

### Why Process Plain Files with Context?
- **User Intent**: Providing context indicates intent to use templating
- **Flexibility**: Allows any file type to be templated when needed
- **Backward Compatibility**: Maintains existing behavior from earlier versions

### Why Always Process Template Extensions?
- **Explicit Intent**: Template extensions clearly indicate files meant for templating
- **Consistency**: Users expect `.tmpl` files to always be processed
- **Error Prevention**: Avoids confusion when templates aren't processed

## Version History
- v1.0 (2025-09-28): Initial requirements documentation
