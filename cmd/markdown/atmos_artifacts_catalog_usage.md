- Build a catalog snapshot from an inventory, then publish it

```
$ atmos artifacts catalog build --repository=shared --input=inventory.yaml --output=catalog.json
$ atmos artifacts catalog publish catalog.json --repository=shared
```

- List or unlist one release in the published catalog

```
$ atmos artifacts catalog add components/terraform/vpc:1.2.0 --repository=shared
$ atmos artifacts catalog remove components/terraform/vpc:1.2.0 --repository=shared
```

- Merge an inventory into the published catalog, keeping every other entry

```
$ atmos artifacts catalog sync --repository=shared --input=inventory.yaml
```
