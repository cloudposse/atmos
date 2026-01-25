## Release 1.470.0

Components PR [#1076](https://github.com/cloudposse/terraform-aws-components/pull/1076)

- Allow specifying elements of `spec.template.spec.kubelet`
- Make taint values optional

The `var.node_pools` map now includes a `kubelet` field that allows specifying elements of `spec.template.spec.kubelet`.
This is useful for configuring the kubelet to use custom settings, such as reserving resources for system daemons.

For more information, see:

- [Karpenter documentation](https://karpenter.sh/docs/concepts/nodepools/#spectemplatespeckubelet)
- [Kubernetes documentation](https://kubernetes.io/docs/reference/config-api/kubelet-config.v1beta1/)

The `value` fields of the `taints` and `startup_taints` lists in the `var.node_pools` map are now optional. This is in
alignment with the Kubernetes API, where `key` and `effect` are required, but the `value` field is optional.
