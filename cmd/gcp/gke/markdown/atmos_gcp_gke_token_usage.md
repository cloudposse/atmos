- Generate a Kubernetes ExecCredential from an explicit identity

```shell
atmos gcp gke token --identity example-deployer
```

- Let Atmos select the inherited or sole configured identity

```shell
atmos gcp gke token
```
