- Uninstall a skill (with confirmation)

```
$ atmos ai skill uninstall terraform-optimizer
```

- Uninstall every installed skill at once

```
$ atmos ai skill uninstall
```

- Force uninstall without confirmation

```
$ atmos ai skill uninstall terraform-optimizer --force
```

- Remove the skill from a specific AI client only

```
$ atmos ai skill uninstall terraform-optimizer --client vscode
```

- Remove the skill from every supported AI client

```
$ atmos ai skill uninstall terraform-optimizer --all-clients
```

- Remove a skill that was installed with --scope user

```
$ atmos ai skill uninstall terraform-optimizer --scope user
```
