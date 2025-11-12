# Config Profiles Test Fixture

This fixture provides a minimal test environment for profile-related functionality.

## Structure

```yaml
config-profiles/
├── atmos.yaml           # Base configuration
└── profiles/            # Profile directories
    ├── developer/       # Developer profile
    │   ├── auth.yaml
    │   └── settings.yaml
    ├── ci/              # CI profile
    │   ├── auth.yaml
    │   └── settings.yaml
    └── production/      # Production profile
        ├── auth.yaml
        └── settings.yaml
```

## Profiles

- **developer**: AWS SSO profile with debug logging and 120-width terminal
- **ci**: GitHub OIDC role with info logging and 80-width terminal
- **production**: Production IAM role with warning logging and 100-width terminal

## Usage

Tests should use "fixtures/scenarios/config-profiles" to switch to this directory.
