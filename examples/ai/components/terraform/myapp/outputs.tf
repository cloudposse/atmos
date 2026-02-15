# MyApp Component Outputs
# These outputs can be queried by the Atmos AI Assistant.

output "app_name" {
  description = "Name of the application"
  value       = var.app_name
}

output "app_version" {
  description = "Version of the application"
  value       = var.app_version
}

output "full_name" {
  description = "Full name including environment"
  value       = local.full_name
}

output "environment" {
  description = "Deployment environment"
  value       = lookup(var.tags, "Environment", "unknown")
}

output "resource_summary" {
  description = "Summary of all resource configurations"
  value       = local.resource_summary
}

output "instance_type" {
  description = "EC2 instance type"
  value       = var.instance_type
}

output "replica_count" {
  description = "Number of replicas"
  value       = var.replica_count
}

output "scaling_config" {
  description = "Autoscaling configuration"
  value = {
    min_replicas = var.min_replicas
    max_replicas = var.max_replicas
  }
}

output "database_config" {
  description = "Database configuration summary"
  value = {
    instance_class = var.db_instance_class
    storage_gb     = var.db_storage_gb
    multi_az       = var.db_multi_az
    encrypted      = var.db_encryption
  }
}

output "cache_config" {
  description = "Cache configuration summary"
  value = {
    node_type          = var.cache_node_type
    num_nodes          = var.cache_num_nodes
    automatic_failover = var.cache_automatic_failover
  }
}

output "security_config" {
  description = "Security configuration summary"
  value = {
    ssl_enabled    = var.ssl_enabled
    waf_enabled    = var.waf_enabled
    public_access  = var.public_access
    db_encryption  = var.db_encryption
  }
}

output "tags" {
  description = "Resource tags"
  value       = var.tags
}
