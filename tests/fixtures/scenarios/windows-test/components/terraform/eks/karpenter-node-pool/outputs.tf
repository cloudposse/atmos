output "node_pools" {
  value       = kubernetes_manifest.node_pool
  description = "Deployed Karpenter NodePool"
}

output "ec2_node_classes" {
  value       = kubernetes_manifest.ec2_node_class
  description = "Deployed Karpenter EC2NodeClass"
}
