# MyApp Component Variables
# These variables are populated from Atmos stack configurations.

# Application Settings
variable "app_name" {
  type        = string
  description = "Name of the application"
}

variable "app_version" {
  type        = string
  description = "Version of the application"
  default     = "1.0.0"
}

# Infrastructure Sizing
variable "instance_type" {
  type        = string
  description = "EC2 instance type for compute resources"
  default     = "t3.small"
}

variable "replica_count" {
  type        = number
  description = "Number of application replicas"
  default     = 1
}

variable "min_replicas" {
  type        = number
  description = "Minimum number of replicas for autoscaling"
  default     = 1
}

variable "max_replicas" {
  type        = number
  description = "Maximum number of replicas for autoscaling"
  default     = 10
}

# Resource Limits
variable "cpu_limit" {
  type        = string
  description = "CPU limit for containers (e.g., '500m', '2000m')"
  default     = "500m"
}

variable "memory_limit" {
  type        = string
  description = "Memory limit for containers (e.g., '512Mi', '4Gi')"
  default     = "512Mi"
}

# Feature Flags
variable "debug_enabled" {
  type        = bool
  description = "Enable debug mode"
  default     = false
}

variable "logging_level" {
  type        = string
  description = "Logging level (debug, info, warn, error)"
  default     = "info"
}

variable "metrics_enabled" {
  type        = bool
  description = "Enable metrics collection"
  default     = true
}

# Networking
variable "public_access" {
  type        = bool
  description = "Allow public access to the application"
  default     = false
}

variable "health_check_path" {
  type        = string
  description = "Path for health check endpoint"
  default     = "/health"
}

variable "health_check_interval" {
  type        = number
  description = "Health check interval in seconds"
  default     = 30
}

# Database Settings
variable "db_instance_class" {
  type        = string
  description = "RDS instance class"
  default     = "db.t3.micro"
}

variable "db_storage_gb" {
  type        = number
  description = "Database storage in GB"
  default     = 20
}

variable "db_multi_az" {
  type        = bool
  description = "Enable Multi-AZ deployment"
  default     = false
}

variable "db_backup_retention" {
  type        = number
  description = "Backup retention period in days"
  default     = 7
}

variable "db_encryption" {
  type        = bool
  description = "Enable database encryption"
  default     = true
}

# Cache Settings
variable "cache_node_type" {
  type        = string
  description = "ElastiCache node type"
  default     = "cache.t3.micro"
}

variable "cache_num_nodes" {
  type        = number
  description = "Number of cache nodes"
  default     = 1
}

variable "cache_automatic_failover" {
  type        = bool
  description = "Enable automatic failover for cache cluster"
  default     = false
}

# Additional Production Settings
variable "ssl_enabled" {
  type        = bool
  description = "Enable SSL/TLS"
  default     = true
}

variable "waf_enabled" {
  type        = bool
  description = "Enable Web Application Firewall"
  default     = false
}

variable "cdn_enabled" {
  type        = bool
  description = "Enable CDN (CloudFront)"
  default     = false
}

# Tags
variable "tags" {
  type        = map(string)
  description = "Tags to apply to all resources"
  default     = {}
}
