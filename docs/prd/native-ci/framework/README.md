# Native CI - Framework

Core CI infrastructure: interfaces, detection, storage, hooks, and configuration.

## Documents

| Document | Description |
|----------|-------------|
| [interfaces.md](./interfaces.md) | Core interfaces: Provider, OutputWriter, Context, Status, Artifact Store, Planfile Store |
| [ci-detection.md](./ci-detection.md) | CI environment detection, `--ci` flag, command parity, lifecycle hooks design |
| [artifact-storage.md](./artifact-storage.md) | Generic `artifact.Store` interface, backends, registry, metadata |
| [hooks-integration.md](./hooks-integration.md) | CI hook commands, lifecycle integration |
| [configuration.md](./configuration.md) | Full `atmos.yaml` schema for planfiles and CI sections |
| [implementation-status.md](./implementation-status.md) | Phases, files to create/modify, sentinel errors, status table, changelog |
