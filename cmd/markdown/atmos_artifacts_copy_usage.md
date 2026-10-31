- Copy an artifact from one repository to another

```
$ atmos artifacts copy components/terraform/vpc:1.2.0 --from=shared --to=backups
```

- Replace a conflicting destination

```
$ atmos artifacts copy components/terraform/vpc:1.2.0 --from=shared --to=backups --force
```
