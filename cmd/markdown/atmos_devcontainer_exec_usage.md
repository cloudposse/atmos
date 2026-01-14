- Execute a command in a devcontainer (non-interactive, output masked)

```
$ atmos devcontainer exec geodesic -- ls -la
```

- Execute in a specific instance

```
$ atmos devcontainer exec terraform --instance my-instance -- terraform version
```

- Run commands that output sensitive data (automatically masked)

```
$ atmos devcontainer exec geodesic -- env | grep AWS
```

- Use interactive mode for full TTY support (tab completion, colors)

```
$ atmos devcontainer exec geodesic --interactive -- bash
$ atmos devcontainer exec geodesic -i -- vim ~/.bashrc
```

- **EXPERIMENTAL**: Use PTY mode for TTY + masking (best of both worlds)

```
$ atmos devcontainer exec geodesic --pty -- bash
$ atmos devcontainer exec geodesic --pty -- env | grep AWS
```

- Disable masking if needed

```
$ atmos devcontainer exec geodesic --mask=false -- cat ~/.aws/config
```

**Automatic Masking**: By default, `exec` runs in non-interactive mode where output is automatically masked based on patterns configured in `atmos.yaml`. This protects sensitive data like AWS keys, GitHub tokens, and other secrets.

Three modes available:
- **Non-interactive** (default): Masking works, but no TTY features
- **Interactive** (`--interactive`): Full TTY support, but masking unavailable (TTY data bypasses Go streams)
- **PTY mode** (`--pty`): Experimental - provides both TTY features AND masking (macOS/Linux only)
