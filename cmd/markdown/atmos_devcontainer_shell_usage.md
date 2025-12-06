- Launch shell in default instance

```
$ atmos devcontainer shell geodesic
```

- Launch shell in named instance

```
$ atmos devcontainer shell terraform --instance alice
```

- Launch shell with custom instance name

```
$ atmos devcontainer shell geodesic --instance prod
```

- Create a new auto-numbered instance (starts new container with unique name)

```
$ atmos devcontainer shell geodesic --new
```

This creates a new instance named "default-1", "default-2", etc. based on the --instance flag (default "default").

- Create a new auto-numbered instance with custom base name

```
$ atmos devcontainer shell geodesic --instance alice --new
```

This creates "alice-1", "alice-2", etc.

- Rebuild existing instance and attach (destroys and recreates container)

```
$ atmos devcontainer shell terraform --replace
```

This rebuilds the "default" instance (or use --instance to specify which one).

- Launch shell and remove container on exit (like 'docker run --rm')

```
$ atmos devcontainer shell geodesic --rm
```

The container will be automatically removed when you exit the shell.
