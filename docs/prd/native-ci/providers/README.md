# Native CI - Providers

CI provider implementations that connect Atmos to specific CI/CD platforms.

## Providers

| Provider | Description |
|----------|-------------|
| [generic.md](./generic.md) | Generic CI provider: detects `CI=true`, provides basic context from env vars with git fallback |
| [github/](./github/) | GitHub Actions provider: status checks, job summaries, PR comments, CI outputs |
