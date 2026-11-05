terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.0"
    }
  }
}
variable "namespace" { type = string }
variable "kubeconfig" { type = string }
provider "kubernetes" {
  config_path = pathexpand(var.kubeconfig)
}
resource "kubernetes_namespace_v1" "demo" {
  metadata { name = var.namespace }
}
output "namespace" { value = kubernetes_namespace_v1.demo.metadata[0].name }
