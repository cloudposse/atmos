- Publish a scaffold to a configured repository

```
$ atmos scaffold publish ./scaffolds/landing-zone \
    --repository=shared --name=scaffolds/landing-zone --version=2.0.0
```

- Take the name and version from the scaffold manifest (metadata.name and metadata.version)

```
$ atmos scaffold publish ./scaffolds/landing-zone --repository=shared
```

- Validate the scaffold and plan the publication without writing anything

```
$ atmos scaffold publish ./scaffolds/landing-zone --repository=shared --dry-run
```

- Attach labels and package documents

```
$ atmos scaffold publish ./scaffolds/landing-zone --repository=shared --version=2.0.0 \
    --labels=team=platform --readme=docs/PACKAGE.md --changelog=docs/CHANGES.md
```

- Generate a project from the published scaffold

```
$ atmos scaffold generate oci://registry.example.com/acme/atmos/scaffolds/landing-zone:2.0.0 ./my-project
```
