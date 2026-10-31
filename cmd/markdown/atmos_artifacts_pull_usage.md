- Pull a version into a directory

```
$ atmos artifacts pull components/terraform/vpc:1.2.0 --repository=shared --output=./vendor/vpc
```

- Replace an existing, different output directory

```
$ atmos artifacts pull components/terraform/vpc:1.2.0 --repository=shared --output=./vendor/vpc --force
```
