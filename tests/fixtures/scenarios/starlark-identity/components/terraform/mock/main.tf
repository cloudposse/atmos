variable "stage" {
  type    = string
  default = "nonprod"
}

variable "enabled" {
  type    = bool
  default = true
}

variable "name" {
  type    = string
  default = ""
}

variable "environment" {
  type    = string
  default = ""
}

variable "region" {
  type    = string
  default = ""
}

variable "message" {
  type    = string
  default = ""
}

variable "foo" {
  type    = string
  default = "foo"
}

variable "bar" {
  type    = string
  default = "bar"
}

variable "baz" {
  type    = string
  default = "baz"
}

variable "tags" {
  type    = map(string)
  default = {}
}

variable "test_list" {
  type    = list(string)
  default = []
}

variable "test_map" {
  type    = map(string)
  default = {}
}

variable "secret_arns_map" {
  description = "Map with keys containing special characters like slashes"
  type        = map(string)
  default = {
    "auth0-event-stream/app/client-id"     = "arn:aws:secretsmanager:us-east-1:123456789012:secret:client-id-abc123"
    "auth0-event-stream/app/client-secret" = "arn:aws:secretsmanager:us-east-1:123456789012:secret:client-secret-xyz789"
  }
}

output "stage" {
  value = var.stage
}

output "foo" {
  value = var.foo
}

output "bar" {
  value = var.bar
}

output "baz" {
  value = var.baz
}

output "tags" {
  value = var.tags
}

output "test_list" {
  value = var.test_list
}

output "test_map" {
  value = var.test_map
}

output "secret_arns_map" {
  description = "Map with keys containing special characters like slashes"
  value       = var.secret_arns_map
}
