# RDS Component - Mock for testing

variable "db_name" {
  type        = string
  default     = "mydb"
  description = "Name of the database"
}

variable "engine" {
  type        = string
  default     = "postgres"
  description = "Database engine"
}

variable "instance_class" {
  type        = string
  default     = "db.t3.medium"
  description = "RDS instance class"
}

output "db_name" {
  value       = var.db_name
  description = "The database name"
}

output "engine" {
  value       = var.engine
  description = "The database engine"
}
