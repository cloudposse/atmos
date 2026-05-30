# Devcontainer Commands Reference

Complete reference for all `atmos devcontainer` subcommands, flags, and usage patterns.

**Note:** The devcontainer feature is experimental.

---

## atmos devcontainer list

List all configured devcontainers and their status.

```shell
atmos devcontainer list
```

---

## atmos devcontainer config

Display the resolved devcontainer configuration.

```shell
atmos devcontainer config <name>
```

### Examples

```shell
atmos devcontainer config default          # Show default devcontainer config
```

---

## atmos devcontainer start

Start a devcontainer, creating it if it does not exist.

```shell
atmos devcontainer start <name> [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `--instance` | Instance name (default: "default") |
| `--attach` | Immediately attach after starting |
| `--identity` | Atmos identity for credential injection |

### Examples

```shell
atmos devcontainer start default
atmos devcontainer start default --attach
atmos devcontainer start default --instance alice
atmos devcontainer start default --identity prod-admin
```

---

## atmos devcontainer stop

Stop a running devcontainer.

```shell
atmos devcontainer stop <name> [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `--instance` | Instance name (default: "default") |
| `--timeout` | Stop timeout duration |
| `--rm` | Remove container after stopping |

### Examples

```shell
atmos devcontainer stop default
atmos devcontainer stop default --rm
atmos devcontainer stop default --timeout 30s
```

---

## atmos devcontainer attach

Attach to a running devcontainer for an interactive shell session.

```shell
atmos devcontainer attach <name> [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `--instance` | Instance name (default: "default") |
| `--identity` | Atmos identity for credential injection |

---

## atmos devcontainer shell

Convenience command: start + attach in one step.

```shell
atmos devcontainer shell [name] [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `--instance` | Instance name (default: "default") |
| `--new` | Always create a new instance |
| `--replace` | Replace existing instance |
| `--rm` | Remove instance on exit |
| `--identity` | Atmos identity for credential injection |
| `--pty` | Enable PTY mode for interactive operations |

### Examples

```shell
atmos devcontainer shell                       # Default devcontainer
atmos devcontainer shell default               # Named devcontainer
atmos devcontainer shell default --new         # Force new instance
atmos devcontainer shell default --rm          # Remove on exit
atmos devcontainer shell default -i prod-admin # With identity
```

---

## atmos devcontainer exec

Execute a command inside a running devcontainer.

```shell
atmos devcontainer exec <name> [flags] -- <command> [args...]
```

### Flags

| Flag | Description |
|------|-------------|
| `--instance` | Instance name (default: "default") |
| `--identity` | Atmos identity for credential injection |

### Examples

```shell
atmos devcontainer exec default -- terraform plan
atmos devcontainer exec default -- aws sts get-caller-identity
atmos devcontainer exec default -- env | grep AWS
atmos devcontainer exec default --instance alice -- bash -c "echo hello"
```

---

## atmos devcontainer rebuild

Destroy and recreate a devcontainer from scratch.

```shell
atmos devcontainer rebuild <name> [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `--instance` | Instance name (default: "default") |

---

## atmos devcontainer remove

Remove a devcontainer and its data.

```shell
atmos devcontainer remove <name> [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `--instance` | Instance name (default: "default") |

---

## atmos devcontainer logs

Show devcontainer logs.

```shell
atmos devcontainer logs <name> [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `--instance` | Instance name (default: "default") |

---

## Configuration Schema

```yaml
devcontainer:
  <name>:
    settings:
      runtime: docker|podman    # Auto-detected if omitted

    spec:
      # Container image
      name: "Display Name"
      image: "org/image:tag"

      # Build from Dockerfile (alternative to image)
      build:
        dockerfile: path/to/Dockerfile
        context: .
        args:
          KEY: value

      # Workspace
      workspaceFolder: "/workspace"
      workspaceMount: "type=bind,source=${WORKSPACE},target=/workspace"

      # Additional mounts
      mounts:
        - "type=bind,source=${HOME}/.aws,target=/root/.aws,readonly"

      # Networking
      forwardPorts:
        - 8080
        - 3000
      portsAttributes:
        8080:
          label: "Web Server"

      # Runtime
      runArgs:
        - "--privileged"
      containerEnv:
        KEY: value
      remoteUser: "root"
```

### Unsupported Spec Fields (Silently Ignored)

| Field | Workaround |
|-------|-----------|
| `features` | Install tools via image or Dockerfile |
| `postCreateCommand` | Use Dockerfile RUN commands |
| `postStartCommand` | Use `atmos devcontainer exec` |
| `customizations` | Not applicable (Atmos is not VS Code) |
| `hostRequirements` | Manage host requirements externally |
