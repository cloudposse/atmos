# Test Fixtures: Scaffolds

This directory contains test fixture scaffolds used for testing the Atmos `init` and `scaffold` commands. These scaffolds test various aspects of the template system, interactive prompts, file generation, and path templating.

## Overview

Each scaffold directory represents a different test case, covering various scenarios and edge cases that the Atmos CLI needs to handle correctly. These fixtures are used by integration tests to verify that:

- Templates are processed correctly
- Interactive forms work as expected
- File paths can be templated
- Different field types are supported
- Conditional file creation works
- Error handling is robust

## Scaffold Types

### Simple Templates (No Interactive Configuration)

#### `default/`
- **Purpose**: Basic template with simple README generation
- **Tests**: Template processing, basic file creation, markdown rendering
- **Features**: 
  - Simple template variable substitution
  - Atmos project structure documentation
  - No interactive configuration required

#### `atmos-yaml/`
- **Purpose**: Tests generation of `atmos.yaml` configuration files
- **Tests**: Configuration file templating, YAML generation
- **Features**:
  - Atmos CLI configuration template
  - No user prompts

#### `editorconfig/`
- **Purpose**: Tests generation of `.editorconfig` files
- **Tests**: Hidden file creation, editor configuration
- **Features**:
  - Dotfile generation
  - Editor configuration setup

#### `gitignore/`
- **Purpose**: Tests generation of `.gitignore` files  
- **Tests**: Git ignore file creation, version control setup
- **Features**:
  - Git configuration files
  - Project-specific ignore patterns

### Demo/Example Templates

#### `demo-stacks/`
- **Purpose**: Demonstrates a complete Atmos project with stacks and components
- **Tests**: Complex project structure, nested directories, Terraform integration
- **Features**:
  - Full Atmos project layout
  - Terraform components (`vpc/main.tf`)
  - Stack configurations
  - Hierarchical stack organization (`orgs/cp/tenant/terraform/dev/us-east-2.yaml`)

#### `demo-helmfile/`
- **Purpose**: Demonstrates Helmfile integration with Atmos
- **Tests**: Helmfile component and stack generation
- **Features**:
  - Helmfile components
  - Kubernetes deployment configurations
  - Stack-based Helm management

#### `demo-localstack/`
- **Purpose**: Demonstrates LocalStack integration for local AWS development
- **Tests**: Docker Compose generation, local development setup
- **Features**:
  - LocalStack configuration
  - Docker Compose setup
  - Local AWS service emulation

### Interactive Configuration Templates

#### `simple-scaffold/`
- **Purpose**: Tests basic interactive field types and form generation
- **Tests**: Interactive prompts, field validation, basic form handling
- **Features**:
  - **Input fields**: `name`, `author` (text input with validation)
  - **Select fields**: `environment` (dropdown with options: dev/staging/prod)
  - **Templated files**: Terraform components in `scaffold/terraform/` directory
  - **Field validation**: Required field handling
  - **Default values**: Pre-populated form fields

#### `rich-project/`
- **Purpose**: Tests comprehensive interactive configuration with all field types
- **Tests**: Complex forms, multiple field types, advanced validation
- **Features**:
  - **Input fields**: `name`, `description`, `author`, `year`, `gcp_project_id`
  - **Select fields**: `license`, `cloud_provider`, `environment`, `terraform_version`
  - **Multi-select fields**: `regions` (multiple AWS regions)
  - **Confirmation fields**: `enable_monitoring`, `enable_logging` (boolean toggles)
  - **Advanced validation**: Complex field interdependencies
  - **Rich defaults**: Sophisticated default value handling

### Advanced Testing Templates

#### `path-test/`
- **Purpose**: Tests dynamic file path templating and conditional file creation
- **Tests**: Path templating, conditional logic, complex directory structures
- **Features**:
  - **Dynamic paths**: `{{.Config.namespace}}-monitoring.yaml`
  - **Nested templated paths**: `{{.Config.namespace}}/{{.Config.subdirectory}}/deep.yaml`
  - **Conditional creation**: Files only created when template variables are provided
  - **Complex field types**: Cloud provider selection with conditional sub-fields
  - **Multi-cloud support**: AWS/GCP specific configurations
  - **Empty value handling**: Optional subdirectory creates different file structures

## Field Type Testing Coverage

| Field Type | Simple Scaffold | Rich Project | Path Test | Purpose |
|------------|----------------|--------------|-----------|---------|
| `input` | ✓ (`name`, `author`) | ✓ (`name`, `description`, `author`, `year`) | ✓ (`namespace`, `author`, `description`) | Text input with validation |
| `select` | ✓ (`environment`) | ✓ (`license`, `cloud_provider`, `environment`, `terraform_version`) | ✓ (`cloud_provider`, `aws_region`, `gcp_region`) | Single selection dropdown |
| `multiselect` | ❌ | ✓ (`regions`) | ❌ | Multiple selection with filtering |
| `confirm` | ❌ | ✓ (`enable_monitoring`, `enable_logging`) | ✓ (`enable_monitoring`) | Boolean yes/no prompts |

## Template Processing Features

### Variable Substitution
- All scaffolds test basic `{{.Config.key}}` template variable replacement
- Advanced scaffolds test conditional logic and complex expressions

### Path Templating  
- `path-test/` specifically tests templated file and directory paths
- Tests handling of empty/optional path segments
- Validates conditional file creation based on variable values

### File Generation
- Tests creation of various file types: `.yaml`, `.tf`, `.md`, `.yml`, `.gitignore`, `.editorconfig`
- Validates proper file permissions and directory structure
- Tests both template files (`.tmpl`) and direct content generation

### Interactive Flows
- Scaffolds with `scaffold.yaml` test the complete interactive form system
- Tests field validation, default values, help text, and error handling
- Validates form flow and user experience

## Usage in Tests

These scaffolds are used by:

1. **Integration Tests**: Full end-to-end testing of `atmos init` and `atmos scaffold generate`
2. **Unit Tests**: Testing specific template processing functions
3. **Interactive Testing**: Manual testing of form flows and user experience
4. **CI/CD**: Automated testing of scaffold functionality across different environments

## Adding New Test Scaffolds

When adding new test scaffolds:

1. Create a new directory under `tests/fixtures/scaffolds/`
2. Include a `README.md` explaining the scaffold's purpose
3. For interactive scaffolds, include a `scaffold.yaml` with field definitions
4. Add template files that test specific features or edge cases
5. Update this README to document the new scaffold's purpose and features
6. Add corresponding test cases that use the new scaffold

## File Structure Convention

```
scaffolds/
├── README.md                    # This file
├── {scaffold-name}/            # Each scaffold in its own directory
│   ├── README.md               # Purpose and usage documentation
│   ├── scaffold.yaml           # Interactive configuration (if needed)
│   ├── atmos.yaml             # Atmos configuration template (if needed)
│   └── {template-files}       # Files to be generated
└── ...
```

This structured approach ensures comprehensive testing coverage of the Atmos template system while providing clear examples for developers and users.