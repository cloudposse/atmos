variable "stage" {
  type = string
}

variable "environment" {
  type = string
}

variable "tenant" {
  type = string
}

variable "foo" {
  type = string
  default = "foo"
}

variable "bar" {
  type = string
  default = "bar"
}

variable "baz" {
  type = string
  default = "baz"
}

# Mock resource for testing
resource "local_file" "mock" {
  content  = jsonencode({
    foo = var.foo
    bar = var.bar
    baz = var.baz
    stage = var.stage
    environment = var.environment
    tenant = var.tenant
  })
  filename = "${path.module}/mock.json"
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

output "stage" {
  value = var.stage
}

output "environment" {
  value = var.environment
}

output "tenant" {
  value = var.tenant
}
