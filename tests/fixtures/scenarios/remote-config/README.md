# Remote Configuration Test Fixtures

This directory contains stable test fixtures served by the mock HTTP server during tests.

## Purpose
These fixtures replace live GitHub URLs in tests to:
- Eliminate fragile dependencies on the live main branch
- Ensure tests are deterministic and reproducible
- Enable offline testing
- Prevent test failures when defaults change in main

## Usage
During tests, the mock HTTP server (started in TestMain) serves this directory at a dynamic URL.
Test fixtures reference these using `http://localhost:8080/` which gets replaced with the actual server URL at runtime.

Test fixtures reference these URLs instead of GitHub:
- Before: `https://raw.githubusercontent.com/cloudposse/atmos/refs/heads/main/atmos.yaml`
- After: `http://localhost:8080/atmos.yaml`

## Maintenance
When intentionally changing defaults or test behavior:
1. Update the relevant files in this directory
2. Run tests to verify changes
3. Commit both the fixture updates and test updates together

## Structure
- `atmos.yaml` - Stable atmos configuration for import tests
- `vendor/` - Content for vendor pull tests
- `stacks/` - Stack configurations for include tests
- `docs/` - Documentation source files for docs generation tests

## Important Note
This directory is served by a mock HTTP server during tests. The content here should be:
- Stable and predictable
- Not dependent on external resources
- Updated only when test requirements change
