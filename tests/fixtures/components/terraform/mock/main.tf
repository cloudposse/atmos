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
