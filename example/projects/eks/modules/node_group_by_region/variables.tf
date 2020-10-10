variable "availability_zones" {
  type = list(string)
}

variable "node_group_size" {
  type = object({
    desired_size = number
    min_size     = number
    max_size     = number
  })
}

variable "cluster_context" {
  type = object({
    cluster_name              = string
    create_before_destroy     = bool
    disk_size                 = number
    enable_cluster_autoscaler = bool
    instance_types            = list(string)
    ami_type                  = string
    ami_release_version       = string
    kubernetes_version        = string
    kubernetes_labels         = map(string)
    kubernetes_taints         = map(string)
    subnet_type_tag_key       = string
    vpc_id                    = string
    resources_to_tag          = list(string)
    module_depends_on         = any
  })
}
