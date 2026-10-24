- Update a single bundled skill if a newer version is available

```
$ atmos ai skill update atmos-terraform
```

- Update every installed bundled skill that has an update available

```
$ atmos ai skill update
```

- Skip the confirmation prompt

```
$ atmos ai skill update --yes
```

- Update and redistribute to a specific AI client

```
$ atmos ai skill update atmos-terraform --client vscode
```

- Update a skill installed to a custom directory

```
$ atmos ai skill update atmos-terraform --path .github/skills
```
