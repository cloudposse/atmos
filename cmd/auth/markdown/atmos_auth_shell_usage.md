- Launch an interactive shell with authentication for the default identity

```
$ atmos auth shell
```

- Launch a shell with a specific identity

```
$ atmos auth shell --identity prod-admin
```

- Launch a shell with a specific shell program

```
$ atmos auth shell --shell /bin/zsh
```

- Pass custom shell arguments using `--`

```
$ atmos auth shell -- -c "env | grep AWS"
```

- Launch a non-login shell

```
$ atmos auth shell -- --norc
```

- Combine identity and shell arguments

```
$ atmos auth shell --identity staging-readonly -- -c "terraform plan"
```
