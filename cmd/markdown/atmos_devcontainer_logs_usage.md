- View logs from the default devcontainer

```
$ atmos devcontainer logs default
```

- View logs from a specific instance

```
$ atmos devcontainer logs terraform --instance my-instance
```

- Follow logs in real-time

```
$ atmos devcontainer logs default --follow
```

- Show only the last 100 lines

```
$ atmos devcontainer logs default --tail 100
```
