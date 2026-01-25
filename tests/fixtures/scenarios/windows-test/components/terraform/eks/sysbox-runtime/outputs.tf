output "enabled" {
  description = "Whether Sysbox is enabled in this environment"
  value       = local.enabled
}

output "runtime_class_name" {
  description = "Name of the created RuntimeClass"
  value       = local.enabled ? kubernetes_manifest.sysbox_runtime_class[0].manifest.metadata.name : null
}

output "runtime_handler" {
  description = "Handler name used by the RuntimeClass (matches containerd config)"
  value       = local.enabled ? var.runtime_handler : null
}

output "node_selector" {
  description = "Node selector labels for Sysbox nodes"
  value       = var.node_selector
}

output "tolerations" {
  description = "Tolerations applied to pods using this RuntimeClass"
  value       = var.tolerations
}
