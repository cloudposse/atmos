- List every artifact in a repository

```
$ atmos artifacts list --repository=shared
```

- List the versions of one artifact

```
$ atmos artifacts list components/terraform/vpc --repository=shared
```

- List across all configured repositories, filtered by labels

```
$ atmos artifacts list --labels=team=platform
```
