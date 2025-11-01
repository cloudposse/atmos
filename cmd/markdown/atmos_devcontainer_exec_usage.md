- Execute a command in the default devcontainer

```
$ atmos devcontainer exec default -- ls -la
```

- Execute in a specific instance

```
$ atmos devcontainer exec terraform --instance my-instance -- terraform version
```

- Run an interactive shell

```
$ atmos devcontainer exec default -- bash
```
