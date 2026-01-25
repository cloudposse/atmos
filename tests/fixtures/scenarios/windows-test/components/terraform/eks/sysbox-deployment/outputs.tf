output "kubernetes_namespace" {
  description = "Kubernetes namespace where the deployment lives"
  value       = local.enabled ? local.namespace_name : null
}

output "deployment_name" {
  description = "Name of the deployment"
  value       = local.enabled ? local.deployment_name : null
}

output "runtime_class_name" {
  description = "RuntimeClass used by the pods"
  value       = local.enabled ? var.runtime_class_name : null
}
