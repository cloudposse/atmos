- Search by text

```
$ atmos artifacts search vpc
```

- Search one repository, filtered by labels, bypassing cached snapshots

```
$ atmos artifacts search vpc --repository=shared --labels='team=platform,purpose=backup' --refresh
```
