- Update kubeconfig with all the required parameters (cluster name and AWS profile/role)

```
 $ atmos aws eks update-kubeconfig --profile=<profile> --name=<cluster_name>
```

- Update kubeconfig with atmos workspace

```
 $ atmos aws eks update-kubeconfig <component> -s <stack>
```

- Override update-kubeconfig atmos params

```
 $ atmos aws eks update-kubeconfig --profile=<profile> --name=<cluster_name> --kubeconfig=<path_to_kubeconfig> --region=us-east-1
```
