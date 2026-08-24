- Launch an interactive shell with authentication for the default identity

```shell
$ atmos auth shell
```

- Launch a shell with a specific identity

```shell
$ atmos auth shell --identity prod-admin
```

- Launch a shell with a specific shell program

```shell
$ atmos auth shell --shell /bin/zsh
```

- Pass custom shell arguments using `--`

```shell
$ atmos auth shell -- -c "env | grep AWS"
```

- Launch a non-login shell

```shell
$ atmos auth shell -- --norc
```

- Combine identity and shell arguments

```shell
$ atmos auth shell --identity staging-readonly -- -c "terraform plan"
```
