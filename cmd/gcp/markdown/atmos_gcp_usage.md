Authenticate a command with a GCP identity linked to a GKE integration:

```shell
atmos auth exec --identity example-deployer -- kubectl get namespaces
```

Emit a Kubernetes exec credential directly:

```shell
atmos gcp gke token --identity example-deployer
```
