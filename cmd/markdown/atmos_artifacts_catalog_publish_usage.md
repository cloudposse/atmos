- Publish a built snapshot to the repository's configured catalog

```
$ atmos artifacts catalog publish catalog.json --repository=shared
```

- Refuse to publish unless the catalog is still the revision the snapshot was built from

```
$ atmos artifacts catalog publish catalog.json --repository=shared --expected-digest=sha256:<digest>
```
