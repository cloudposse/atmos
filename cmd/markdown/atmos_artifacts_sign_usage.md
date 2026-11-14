- Sign an immutable digest reference

```
$ atmos artifacts sign 'components/terraform/vpc@sha256:<digest>' --repository=shared
```

- Accept Sigstore transparency log publication in CI

```
$ atmos artifacts sign 'components/terraform/vpc@sha256:<digest>' --repository=shared --yes
```
