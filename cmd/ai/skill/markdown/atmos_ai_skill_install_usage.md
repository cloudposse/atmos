- Install an official skill by name (offline, no network)

```
$ atmos ai skill install atmos-terraform
```

- Install all Atmos agent skills

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
