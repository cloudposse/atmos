- Start the default devcontainer

```
$ atmos devcontainer start default
```

- Start and attach to the container

```
$ atmos devcontainer start default --attach
```

- Start a specific instance

```
$ atmos devcontainer start terraform --instance my-instance
```

- Start with custom runtime

```
$ export ATMOS_CONTAINER_RUNTIME=podman
$ atmos devcontainer start default
```
