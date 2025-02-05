# Atmos Test Fixtures

This directory contains test fixtures for Atmos CLI testing. The structure is organized to maximize component reuse and reduce duplication through Atmos's inheritance and vendoring capabilities.

## Directory Structure

```
fixtures/
├── atmos.yaml              # Base configuration file
├── components/             # Shared reusable components
│   └── terraform/         # Terraform modules used across tests
├── scenarios/             # Test scenarios
│   ├── mock-terraform/    # Mock Terraform testing
│   ├── vendor/           # Vendored components
│   └── ...               # Other test scenarios
└── schemas/               # JSON schemas for validation
```

## Component Reuse

Components are managed in two ways:
1. **Direct Reference**: Using `base_path` to point to the shared components directory
2. **Vendoring**: Using Atmos's vendor capability to vendor components into specific test scenarios

## Configuration Inheritance

Test scenarios inherit from the base `atmos.yaml` and override only necessary settings. This reduces duplication and makes test maintenance easier.

## Usage

1. For new test scenarios, create a new directory under `scenarios/`
2. Inherit from the base configuration using:
   ```yaml
   inherits:
     - ../../atmos.yaml
   ```
3. Override only the necessary settings for your test case

## Best Practices

- Use vendoring for scenarios that need isolated components
- Leverage inheritance to reduce configuration duplication
- Keep shared components in the central `components/` directory
- Document scenario-specific requirements in a README within each scenario directory 