output "id" {
  value       = join("", aws_elasticache_replication_group.default[*].id)
  description = "Redis cluster ID"
}

output "security_group_id" {
  value       = module.aws_security_group.id
  description = "The ID of the created security group"
}

output "security_group_name" {
  value       = module.aws_security_group.name
  description = "The name of the created security group"
}

output "port" {
  value       = var.port
  description = "Redis port"
}

output "endpoint" {
  value       = local.endpoint_address
  description = "Redis primary, configuration or serverless endpoint , whichever is appropriate for the given configuration"
}

output "reader_endpoint_address" {
  value       = local.reader_endpoint_address
  description = "The address of the endpoint for the reader node in the replication group, if the cluster mode is disabled or serverless is being used."
}


output "member_clusters" {
  value       = aws_elasticache_replication_group.default[*].member_clusters
  description = "Redis cluster members"
}

output "host" {
  value       = module.dns.hostname
  description = "Redis hostname"
}

output "arn" {
  value       = local.arn
  description = "Elasticache Replication Group ARN"
}

output "engine_version_actual" {
  value       = join("", aws_elasticache_replication_group.default[*].engine_version_actual)
  description = "The running version of the cache engine"
}

output "cluster_enabled" {
  value       = join("", aws_elasticache_replication_group.default[*].cluster_enabled)
  description = "Indicates if cluster mode is enabled"
}

output "serverless_enabled" {
  value       = var.serverless_enabled
  description = "Indicates if serverless mode is enabled"
}

output "transit_encryption_mode" {
  value       = join("", aws_elasticache_replication_group.default[*].transit_encryption_mode)
  description = "The transit encryption mode of the replication group"
}
