- Update kubeconfig with all the required parameters (cluster name and AWS profile/role)

```shell
 $ atmos aws eks update-kubeconfig --profile=<profile> --name=<cluster_name>
```

- Update kubeconfig with atmos workspace

```shell
 $ atmos aws eks update-kubeconfig <component> -s <stack>
```

- Override update-kubeconfig atmos params

```shell
 $ atmos aws eks update-kubeconfig --profile=<profile> --name=<cluster_name> --kubeconfig=<path_to_kubeconfig> --region=us-east-1
```
