- Install an official skill by name (offline, no network)

```
$ atmos ai skill install atmos-terraform
```

- Install every bundled skill at once (offline, no network)

```
$ atmos ai skill install
```

- Install all Atmos agent skills from the GitHub repository instead of the embedded catalog

```
$ atmos ai skill install cloudposse/atmos
```

- Install with a specific version

```
$ atmos ai skill install cloudposse/atmos@v1.200.0
```

- Install a third-party skill

```
$ atmos ai skill install yourorg/your-skill
```

- Force reinstall (overwrite existing installation)

```
$ atmos ai skill install cloudposse/atmos --force
```

- Skip confirmation prompt

```
$ atmos ai skill install cloudposse/atmos --yes
```

- Install to a custom directory (e.g. for VS Code/Copilot, skips auto-distribution)

```
$ atmos ai skill install atmos-terraform --path .github/skills
```

- Distribute the skill to a specific AI client

```
$ atmos ai skill install atmos-terraform --client vscode
```

- Distribute the skill to every supported AI client

```
$ atmos ai skill install atmos-terraform --all-clients
```

- Distribute into each client's personal, user-level skill directory instead of the project

```
$ atmos ai skill install atmos-terraform --scope user
```

- Alias for --scope user

```
$ atmos ai skill install atmos-terraform --global
```
